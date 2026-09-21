//go:build integration

package repositories_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/infra/db"
	"crowdbeats/internal/infra/db/repositories"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if err := validateIntegrationTarget(databaseURL, os.Getenv("DATABASE_URL"), os.Getenv("ALLOW_INTEGRATION_DB_RESET") == "true"); err != nil {
		t.Fatal(err)
	}

	pool, err := db.NewPool(context.Background(), databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	deadline := time.Now().Add(10 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = pool.Ping(pingCtx)
		cancel()
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			require.NoError(t, err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	resetCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, db.RunMigrations(resetCtx, pool))
	_, err = pool.Exec(resetCtx, `
		truncate table room_events, room_queue, votes, room_tracks, spotify_tracks, user_sessions, room_qr_codes, rooms restart identity cascade;
		alter sequence room_track_fifo_seq restart with 1;
	`)
	require.NoError(t, err)
	return pool
}

// The integration suite runs migrations and TRUNCATE. Keep both guards even
// when Compose or CI supplies a dedicated database.
func validateIntegrationTarget(testURL, developmentURL string, allowReset bool) error {
	if testURL == "" {
		return errors.New("TEST_DATABASE_URL is required for integration tests")
	}
	if !allowReset {
		return errors.New("ALLOW_INTEGRATION_DB_RESET=true is required for destructive integration tests")
	}
	if testURL == developmentURL {
		return errors.New("TEST_DATABASE_URL must differ from DATABASE_URL")
	}
	config, err := pgx.ParseConfig(testURL)
	if err != nil || !strings.Contains(strings.ToLower(config.Database), "test") {
		return errors.New("TEST_DATABASE_URL must target a database whose name contains test")
	}
	return nil
}

func TestIntegrationTargetGuard(t *testing.T) {
	valid := "postgres://test_user:test_password@localhost:55432/crowdbeats_test?sslmode=disable"
	for _, tc := range []struct {
		name, testURL, developmentURL string
		allow                         bool
		wantError                     bool
	}{
		{"missing URL", "", "", true, true},
		{"missing acknowledgement", valid, "", false, true},
		{"same as development", valid, valid, true, true},
		{"non-test database", "postgres://test_user:test_password@localhost:5432/crowdbeats?sslmode=disable", "", true, true},
		{"dedicated database", valid, "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIntegrationTarget(tc.testURL, tc.developmentURL, tc.allow)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
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
