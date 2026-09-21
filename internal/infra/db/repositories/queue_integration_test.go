//go:build integration

package repositories_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/infra/db"
	"crowdbeats/internal/infra/db/repositories"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// These tests use testPool, which truncates its database. Compile them until
// Prompt 5 provides an explicitly isolated TEST_DATABASE_URL.
type integrationBroadcaster struct{}

func (integrationBroadcaster) Broadcast(usecase.LiveEvent)            {}
func (integrationBroadcaster) DisconnectSession(uuid.UUID, uuid.UUID) {}

func queueServices(pool *pgxpool.Pool) *usecase.Services {
	store := db.NewStore(pool)
	return usecase.NewServices(store, store, nil, integrationBroadcaster{}, nil, nil)
}

func queueTrack(t *testing.T, pool *pgxpool.Pool, roomID, sessionID uuid.UUID, number int) uuid.UUID {
	t.Helper()
	var catalogID, trackID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		insert into spotify_tracks (spotify_track_id, title, artist_names)
		values ($1, $2, 'Artist') returning id
	`, fmt.Sprintf("queue-track-%02d", number), fmt.Sprintf("Song %d", number)).Scan(&catalogID)
	require.NoError(t, err)
	err = pool.QueryRow(context.Background(), `
		insert into room_tracks (room_id, spotify_track_ref_id, proposed_by_session_id)
		values ($1, $2, $3) returning id
	`, roomID, catalogID, sessionID).Scan(&trackID)
	require.NoError(t, err)
	return trackID
}

func queueSession(t *testing.T, repos *repositories.Set, roomID uuid.UUID, number int) session.Session {
	t.Helper()
	created, err := repos.Sessions().Create(context.Background(), session.CreateInput{
		RoomID: roomID, SessionTokenHash: fmt.Sprintf("queue-session-%d", number),
		Nickname: fmt.Sprintf("Guest %d", number), Role: session.RoleGuest,
	})
	require.NoError(t, err)
	return created
}

func TestQueueSnapshotOrdersVotesThenFIFOWithSeparateNowPlaying(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)
	created, err := repos.Rooms().Create(context.Background(), room.CreateInput{
		Name: "Queue", Status: room.StatusActive, QueueLimit: 2,
		MaxVotesPerUser: 5, QRTTLSeconds: 180, RecalcIntervalSeconds: 2, ManagerSecretHash: "hash",
	})
	require.NoError(t, err)
	first := queueSession(t, repos, created.ID, 1)
	second := queueSession(t, repos, created.ID, 2)
	playing := queueTrack(t, pool, created.ID, first.ID, 1)
	older := queueTrack(t, pool, created.ID, first.ID, 2)
	highest := queueTrack(t, pool, created.ID, first.ID, 3)
	newer := queueTrack(t, pool, created.ID, first.ID, 4)
	require.NoError(t, repos.Votes().Insert(context.Background(), created.ID, older, first.ID))
	require.NoError(t, repos.Votes().Insert(context.Background(), created.ID, highest, first.ID))
	require.NoError(t, repos.Votes().Insert(context.Background(), created.ID, highest, second.ID))
	require.NoError(t, repos.Votes().Insert(context.Background(), created.ID, newer, first.ID))
	services := queueServices(pool)
	require.NoError(t, services.SetTrackPlaying(context.Background(), created.ID, playing))
	queuedCount, err := repos.Tracks().CountQueued(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, 3, queuedCount, "playing must not consume a queued slot")
	changed, err := services.RecalculateRoomQueue(context.Background(), created.ID)
	require.NoError(t, err)
	require.True(t, changed)
	snapshot, err := repos.Queue().LoadSnapshot(context.Background(), created.ID)
	require.NoError(t, err)
	require.NotNil(t, snapshot.NowPlaying)
	require.Equal(t, playing, snapshot.NowPlaying.RoomTrackID)
	require.Len(t, snapshot.Items, 2)
	require.Equal(t, highest, snapshot.Items[0].RoomTrackID)
	require.Equal(t, older, snapshot.Items[1].RoomTrackID, "equal scores must use FIFO")
	require.Equal(t, 1, snapshot.Items[0].Position)
	require.Equal(t, 2, snapshot.Items[1].Position)
	changed, err = services.RecalculateRoomQueue(context.Background(), created.ID)
	require.NoError(t, err)
	require.False(t, changed, "unchanged public snapshot must not emit a new version")
}

func TestConcurrentVoteAndRecalculationsPreserveVoteCache(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)
	created := integrationRoom(t, repos)
	guest := queueSession(t, repos, created.ID, 1)
	trackID := queueTrack(t, pool, created.ID, guest.ID, 1)
	services := queueServices(pool)
	start := make(chan struct{})
	errors := make(chan error, 3)
	var group sync.WaitGroup
	group.Add(3)
	for i := 0; i < 2; i++ {
		go func() {
			defer group.Done()
			<-start
			_, err := services.RecalculateRoomQueue(context.Background(), created.ID)
			errors <- err
		}()
	}
	go func() {
		defer group.Done()
		<-start
		_, err := services.AddVote(context.Background(), created.ID, guest, trackID)
		errors <- err
	}()
	close(start)
	group.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	// The next batch must observe the vote regardless of transaction order.
	_, err := services.RecalculateRoomQueue(context.Background(), created.ID)
	require.NoError(t, err)
	changed, err := services.RecalculateRoomQueue(context.Background(), created.ID)
	require.NoError(t, err)
	require.False(t, changed, "the latest serialized snapshot version must be selected")
	var cached int
	require.NoError(t, pool.QueryRow(context.Background(), `select vote_count_cached from room_tracks where id = $1`, trackID).Scan(&cached))
	require.Equal(t, 1, cached)
	snapshot, err := repos.Queue().LoadSnapshot(context.Background(), created.ID)
	require.NoError(t, err)
	require.Nil(t, snapshot.NowPlaying)
	require.Len(t, snapshot.Items, 1)
	require.Equal(t, 1, snapshot.Items[0].VoteCount)
	var duplicatePositions int
	require.NoError(t, pool.QueryRow(context.Background(), `
		select count(*) from (select position from room_queue where room_id = $1 group by position having count(*) > 1) p
	`, created.ID).Scan(&duplicatePositions))
	require.Zero(t, duplicatePositions)
}

func TestConcurrentMarkPlayingLeavesOnePlayingTrack(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)
	created := integrationRoom(t, repos)
	guest := queueSession(t, repos, created.ID, 1)
	a := queueTrack(t, pool, created.ID, guest.ID, 1)
	b := queueTrack(t, pool, created.ID, guest.ID, 2)
	services := queueServices(pool)
	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, id := range []uuid.UUID{a, b} {
		go func(id uuid.UUID) {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			errors <- services.SetTrackPlaying(ctx, created.ID, id)
		}(id)
	}
	close(start)
	require.NoError(t, <-errors)
	require.NoError(t, <-errors)
	var count int
	require.NoError(t, pool.QueryRow(context.Background(), `select count(*) from room_tracks where room_id = $1 and status = 'playing'`, created.ID).Scan(&count))
	require.Equal(t, 1, count)
}
