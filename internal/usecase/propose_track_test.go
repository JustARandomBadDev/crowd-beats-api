package usecase_test

import (
	"context"
	"net/http"
	"testing"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	domainspotify "crowdbeats/internal/domain/spotify"
	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestProposeTrackReturnsDuplicateWhenTrackAlreadyActiveInRoom(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	sessionID := uuid.New()
	catalogID := uuid.New()
	existingTrackID := uuid.New()

	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, Status: room.StatusActive, QueueLimit: 20}, nil
	}
	h.Repos.TrackRepo.CountActiveFn = func(context.Context, uuid.UUID) (int, error) {
		return 1, nil
	}
	h.Repos.SpotifyRepo.GetBySpotifyIDFn = func(context.Context, string) (string, domainspotify.Track, error) {
		return catalogID.String(), domainspotify.Track{
			SpotifyTrackID: "spotify:1",
			Title:          "Hey Ya!",
			ArtistNames:    "Outkast",
		}, nil
	}
	h.Repos.TrackRepo.CreateQueuedFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (track.RoomTrack, error) {
		return track.RoomTrack{}, &pgconn.PgError{Code: "23505", ConstraintName: "uq_room_tracks_active_unique_track"}
	}
	h.Repos.TrackRepo.GetActiveDuplicateFn = func(context.Context, uuid.UUID, uuid.UUID) (track.DuplicateInfo, error) {
		position := 2
		return track.DuplicateInfo{
			ID:               existingTrackID,
			CurrentVoteCount: 4,
			Position:         &position,
		}, nil
	}

	resp, status, err := h.Services.ProposeTrack(context.Background(), roomID, session.Session{ID: sessionID}, "spotify:1")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.Equal(t, true, resp["duplicate"])

	existing := resp["existing_room_track"].(map[string]any)
	require.Equal(t, existingTrackID, existing["id"])
	require.Equal(t, 4, existing["current_vote_count"])
	require.Empty(t, h.Broadcaster.Events)
}
