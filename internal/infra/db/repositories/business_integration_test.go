//go:build integration

package repositories_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/infra/db"
	"crowdbeats/internal/infra/db/repositories"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func integrationRoom(t *testing.T, repos *repositories.Set) room.Room {
	t.Helper()
	created, err := repos.Rooms().Create(context.Background(), room.CreateInput{
		Name: "Bar", Status: room.StatusActive, QueueLimit: 20,
		MaxVotesPerUser: 5, QRTTLSeconds: 180, RecalcIntervalSeconds: 10,
		ManagerSecretHash: "hash",
	})
	require.NoError(t, err)
	return created
}

func TestPresenceCountsOnlyRecentlySeenActiveSessions(t *testing.T) {
	pool := testPool(t) // Existing integration helper truncates its target DB; do not run without isolation.
	repos := repositories.NewSet(pool)
	createdRoom := integrationRoom(t, repos)
	create := func(hash string) session.Session {
		created, err := repos.Sessions().Create(context.Background(), session.CreateInput{
			RoomID: createdRoom.ID, SessionTokenHash: hash, Nickname: hash, Role: session.RoleGuest,
		})
		require.NoError(t, err)
		return created
	}
	create("recent")
	stale := create("stale")
	left := create("left")
	_, err := pool.Exec(context.Background(), `update user_sessions set last_seen_at = now() - interval '3 minutes' where id = $1`, stale.ID)
	require.NoError(t, err)
	require.NoError(t, repos.Sessions().Leave(context.Background(), left.ID))

	count, err := repos.Rooms().CountActiveUsers(context.Background(), createdRoom.ID)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	stats, err := repos.Rooms().LoadStats(context.Background(), createdRoom.ID)
	require.NoError(t, err)
	require.Equal(t, 1, stats.ActiveUsers)

	require.NoError(t, repos.Sessions().Heartbeat(context.Background(), stale.ID))
	count, err = repos.Rooms().CountActiveUsers(context.Background(), createdRoom.ID)
	require.NoError(t, err)
	require.Equal(t, 2, count)
	err = repos.Sessions().Heartbeat(context.Background(), left.ID)
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestQRRotationRevokesPreviousCodeAndRejectsExpiredCode(t *testing.T) {
	pool := testPool(t)
	store := db.NewStore(pool)
	createdRoom := integrationRoom(t, repositories.NewSet(pool))
	rotate := func(code string) {
		err := store.Run(context.Background(), func(repos usecase.RepositorySet) error {
			if _, err := repos.Rooms().GetByIDForUpdate(context.Background(), createdRoom.ID); err != nil {
				return err
			}
			return repos.Rooms().RotateQRCode(context.Background(), createdRoom.ID, code, time.Now().Add(time.Hour))
		})
		require.NoError(t, err)
	}
	rotate("qr_first_active")
	rotate("qr_second_active")
	_, err := store.Rooms().GetByQRCode(context.Background(), "qr_first_active")
	require.ErrorIs(t, err, pgx.ErrNoRows)
	_, err = store.Rooms().GetByQRCode(context.Background(), "qr_second_active")
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		insert into room_qr_codes (room_id, code, expires_at) values ($1, 'qr_expired_code', now() - interval '1 minute')
	`, createdRoom.ID)
	require.NoError(t, err)
	_, err = store.Rooms().GetByQRCode(context.Background(), "qr_expired_code")
	require.ErrorIs(t, err, pgx.ErrNoRows)

	_, err = pool.Exec(context.Background(), `
		insert into room_qr_codes (room_id, code, expires_at, revoked) values ($1, 'qr_revoked_code', now() + interval '1 hour', true)
	`, createdRoom.ID)
	require.NoError(t, err)
	_, err = store.Rooms().GetByQRCode(context.Background(), "qr_revoked_code")
	require.ErrorIs(t, err, pgx.ErrNoRows)
}

func TestConcurrentQRRotationLeavesExactlyOneActiveCode(t *testing.T) {
	pool := testPool(t)
	store := db.NewStore(pool)
	createdRoom := integrationRoom(t, repositories.NewSet(pool))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, code := range []string{"qr_parallel_one", "qr_parallel_two"} {
		go func(code string) {
			<-start
			results <- store.Run(ctx, func(repos usecase.RepositorySet) error {
				if _, err := repos.Rooms().GetByIDForUpdate(ctx, createdRoom.ID); err != nil {
					return err
				}
				return repos.Rooms().RotateQRCode(ctx, createdRoom.ID, code, time.Now().Add(time.Hour))
			})
		}(code)
	}
	close(start)
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	var count int
	err := pool.QueryRow(context.Background(), `
		select count(*) from room_qr_codes where room_id = $1 and revoked = false and expires_at > now()
	`, createdRoom.ID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestConcurrentTrackInsertReturnsCreatedAndDuplicateWithoutAbortingTransaction(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)
	roomID, sessionID, _, catalogID := seedRoomSessionAndCatalog(t, pool, repos)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type outcome struct {
		id      uuid.UUID
		created bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			tx, err := pool.Begin(ctx)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			defer tx.Rollback(context.Background())
			locked := repositories.NewSet(tx)
			inserted, created, err := locked.Tracks().CreateQueuedIfAbsent(ctx, roomID, catalogID, sessionID)
			if err != nil {
				results <- outcome{err: err}
				return
			}
			id := inserted.ID
			if !created {
				duplicate, err := locked.Tracks().GetActiveDuplicate(ctx, roomID, catalogID)
				if err != nil {
					results <- outcome{err: err}
					return
				}
				id = duplicate.ID
			}
			err = tx.Commit(ctx)
			results <- outcome{id: id, created: created, err: err}
		}()
	}
	close(start)
	a, b := <-results, <-results
	require.NoError(t, a.err)
	require.NoError(t, b.err)
	require.NotEqual(t, a.created, b.created)
	require.Equal(t, a.id, b.id)
	var count int
	err := pool.QueryRow(context.Background(), `
		select count(*) from room_tracks where room_id = $1 and spotify_track_ref_id = $2 and status in ('queued','playing')
	`, roomID, catalogID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count)
}

func TestSessionSwitchRollbackPreservesOldPresenceAndHistory(t *testing.T) {
	pool := testPool(t)
	store := db.NewStore(pool)
	repos := repositories.NewSet(pool)
	oldRoom := integrationRoom(t, repos)
	newRoom := integrationRoom(t, repos)
	oldSession, err := repos.Sessions().Create(context.Background(), session.CreateInput{
		RoomID: oldRoom.ID, SessionTokenHash: "old-token-hash", Nickname: "Old", Role: session.RoleGuest,
	})
	require.NoError(t, err)
	var catalogID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		insert into spotify_tracks (spotify_track_id, title, artist_names) values ($1, 'Song', 'Artist') returning id
	`, "0123456789ABCDEFGHIJKL").Scan(&catalogID)
	require.NoError(t, err)
	oldTrack, err := repos.Tracks().CreateQueued(context.Background(), oldRoom.ID, catalogID, oldSession.ID)
	require.NoError(t, err)
	require.NoError(t, repos.Votes().Insert(context.Background(), oldRoom.ID, oldTrack.ID, oldSession.ID))
	rollbackErr := errors.New("force rollback")
	err = store.Run(context.Background(), func(txRepos usecase.RepositorySet) error {
		if err := txRepos.Sessions().Leave(context.Background(), oldSession.ID); err != nil {
			return err
		}
		_, err := txRepos.Sessions().Create(context.Background(), session.CreateInput{
			RoomID: newRoom.ID, SessionTokenHash: "new-token-hash", Nickname: "New", Role: session.RoleGuest,
		})
		if err != nil {
			return err
		}
		return rollbackErr
	})
	require.ErrorIs(t, err, rollbackErr)
	oldAfter, err := repos.Sessions().GetByTokenHash(context.Background(), "old-token-hash")
	require.NoError(t, err)
	require.Equal(t, oldRoom.ID, oldAfter.RoomID)
	require.Equal(t, "Old", oldAfter.Nickname)
	require.Equal(t, session.StatusActive, oldAfter.Status)
	_, err = repos.Sessions().GetByTokenHash(context.Background(), "new-token-hash")
	require.ErrorIs(t, err, pgx.ErrNoRows)

	err = store.Run(context.Background(), func(txRepos usecase.RepositorySet) error {
		if err := txRepos.Sessions().Leave(context.Background(), oldSession.ID); err != nil {
			return err
		}
		_, err := txRepos.Sessions().Create(context.Background(), session.CreateInput{
			RoomID: newRoom.ID, SessionTokenHash: "new-token-hash", Nickname: "New", Role: session.RoleGuest,
		})
		return err
	})
	require.NoError(t, err)
	oldAfter, err = repos.Sessions().GetByTokenHash(context.Background(), "old-token-hash")
	require.NoError(t, err)
	require.Equal(t, oldRoom.ID, oldAfter.RoomID)
	require.Equal(t, "Old", oldAfter.Nickname)
	require.Equal(t, session.StatusLeft, oldAfter.Status)
	newSession, err := repos.Sessions().GetByTokenHash(context.Background(), "new-token-hash")
	require.NoError(t, err)
	require.Equal(t, newRoom.ID, newSession.RoomID)
	require.NotEqual(t, oldSession.ID, newSession.ID)
	var historicalVotes int
	err = pool.QueryRow(context.Background(), `select count(*) from votes where session_id = $1 and room_id = $2`, oldSession.ID, oldRoom.ID).Scan(&historicalVotes)
	require.NoError(t, err)
	require.Equal(t, 1, historicalVotes)
}
