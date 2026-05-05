//go:build integration

package repositories_test

import (
	"context"
	"os"
	"testing"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/infra/db"
	"crowdbeats/internal/infra/db/repositories"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	pool, err := db.NewPool(context.Background(), databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	deadline := time.Now().Add(10 * time.Second)
	for {
		err = pool.Ping(context.Background())
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			require.NoError(t, err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	require.NoError(t, db.RunMigrations(context.Background(), pool))
	_, err = pool.Exec(context.Background(), `
		truncate table room_events, room_queue, votes, room_tracks, spotify_tracks, user_sessions, room_qr_codes, rooms restart identity cascade;
		alter sequence room_track_fifo_seq restart with 1;
	`)
	require.NoError(t, err)
	return pool
}

func TestRoomRepositoryCreateAndGetByID(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)

	created, err := repos.Rooms().Create(context.Background(), room.CreateInput{
		Name:                  "Le Neon",
		Status:                room.StatusActive,
		QueueLimit:            20,
		MaxVotesPerUser:       5,
		QRTTLSeconds:          14400,
		RecalcIntervalSeconds: 10,
		ManagerSecretHash:     "hash",
	})
	require.NoError(t, err)

	found, err := repos.Rooms().GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, "Le Neon", found.Name)
	require.Equal(t, room.StatusActive, found.Status)
}

func TestSessionRepositoryCreateAndGetByTokenHash(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)

	createdRoom, err := repos.Rooms().Create(context.Background(), room.CreateInput{
		Name:                  "Bar",
		Status:                room.StatusActive,
		QueueLimit:            20,
		MaxVotesPerUser:       5,
		QRTTLSeconds:          14400,
		RecalcIntervalSeconds: 10,
		ManagerSecretHash:     "hash",
	})
	require.NoError(t, err)

	createdSession, err := repos.Sessions().Create(context.Background(), session.CreateInput{
		RoomID:           createdRoom.ID,
		SessionTokenHash: "token-hash",
		Nickname:         "alex",
		Role:             session.RoleGuest,
	})
	require.NoError(t, err)

	found, err := repos.Sessions().GetByTokenHash(context.Background(), "token-hash")
	require.NoError(t, err)
	require.Equal(t, createdSession.ID, found.ID)
	require.Equal(t, createdRoom.ID, found.RoomID)
}

func TestVoteRepositoryUniqueConstraint(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)

	roomID, sessionID, roomTrackID := seedRoomSessionAndTrack(t, pool, repos)

	err := repos.Votes().Insert(context.Background(), roomID, roomTrackID, sessionID)
	require.NoError(t, err)

	err = repos.Votes().Insert(context.Background(), roomID, roomTrackID, sessionID)
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23505", pgErr.Code)
	require.Equal(t, "uq_votes_session_track", pgErr.ConstraintName)
}

func TestTrackRepositoryActiveDuplicateConstraint(t *testing.T) {
	pool := testPool(t)
	repos := repositories.NewSet(pool)

	_, sessionID, _, spotifyTrackRefID := seedRoomSessionAndCatalog(t, pool, repos)
	roomID := currentRoomID(t, pool)

	_, err := repos.Tracks().CreateQueued(context.Background(), roomID, spotifyTrackRefID, sessionID)
	require.NoError(t, err)

	_, err = repos.Tracks().CreateQueued(context.Background(), roomID, spotifyTrackRefID, sessionID)
	require.Error(t, err)

	var pgErr *pgconn.PgError
	require.ErrorAs(t, err, &pgErr)
	require.Equal(t, "23505", pgErr.Code)
	require.Equal(t, "uq_room_tracks_active_unique_track", pgErr.ConstraintName)
}

func seedRoomSessionAndTrack(t *testing.T, pool *pgxpool.Pool, repos *repositories.Set) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	roomID, sessionID, _, spotifyTrackRefID := seedRoomSessionAndCatalog(t, pool, repos)
	createdTrack, err := repos.Tracks().CreateQueued(context.Background(), roomID, spotifyTrackRefID, sessionID)
	require.NoError(t, err)
	return roomID, sessionID, createdTrack.ID
}

func seedRoomSessionAndCatalog(t *testing.T, pool *pgxpool.Pool, repos *repositories.Set) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	createdRoom, err := repos.Rooms().Create(context.Background(), room.CreateInput{
		Name:                  "Test Room",
		Status:                room.StatusActive,
		QueueLimit:            20,
		MaxVotesPerUser:       5,
		QRTTLSeconds:          14400,
		RecalcIntervalSeconds: 10,
		ManagerSecretHash:     "hash",
	})
	require.NoError(t, err)

	createdSession, err := repos.Sessions().Create(context.Background(), session.CreateInput{
		RoomID:           createdRoom.ID,
		SessionTokenHash: "session-hash",
		Nickname:         "alex",
		Role:             session.RoleGuest,
	})
	require.NoError(t, err)

	var spotifyTrackRefID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		insert into spotify_tracks (spotify_track_id, title, artist_names, album_name, duration_ms, uri, raw_payload, created_at, updated_at)
		values ($1, $2, $3, $4, $5, $6, '{}'::jsonb, $7, $7)
		returning id
	`, "spotify-id-1", "Hey Ya!", "Outkast", "Speakerboxxx", 235213, "spotify:track:1", time.Now().UTC()).Scan(&spotifyTrackRefID)
	require.NoError(t, err)

	return createdRoom.ID, createdSession.ID, createdRoom.ID, spotifyTrackRefID
}

func currentRoomID(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var roomID uuid.UUID
	err := pool.QueryRow(context.Background(), `select id from rooms limit 1`).Scan(&roomID)
	require.NoError(t, err)
	return roomID
}
