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
	"crowdbeats/internal/usecase"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

const validTrackID = "0123456789ABCDEFGHIJKL"

func proposeHarness(t *testing.T, limit, queuedCount int) (*testutil.Harness, uuid.UUID, session.Session, uuid.UUID) {
	t.Helper()
	h := testutil.NewHarness()
	roomID, catalogID := uuid.New(), uuid.New()
	current := session.Session{ID: uuid.New(), RoomID: roomID, Status: session.StatusActive}
	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, Status: room.StatusActive, QueueLimit: limit}, nil
	}
	h.Repos.SessionRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (session.Session, error) {
		return current, nil
	}
	h.Repos.SpotifyRepo.GetBySpotifyIDFn = func(context.Context, string) (string, domainspotify.Track, error) {
		return catalogID.String(), domainspotify.Track{SpotifyTrackID: validTrackID, Title: "Song"}, nil
	}
	h.Repos.TrackRepo.CountQueuedFn = func(context.Context, uuid.UUID) (int, error) {
		return queuedCount, nil
	}
	return h, roomID, current, catalogID
}

func requireProposeError(t *testing.T, err error, code string, status int) {
	t.Helper()
	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, code, apiErr.Code)
	require.Equal(t, status, apiErr.Status)
}

func TestProposeTrackRejectsInvalidIDBeforeSpotify(t *testing.T) {
	h := testutil.NewHarness()
	called := false
	h.Repos.SpotifyProvider.GetTrackFn = func(context.Context, string) (domainspotify.Track, error) {
		called = true
		return domainspotify.Track{}, nil
	}
	_, _, err := h.Services.ProposeTrack(context.Background(), uuid.New(), session.Session{}, "spotify:1")
	requireProposeError(t, err, "INVALID_SPOTIFY_TRACK_ID", http.StatusBadRequest)
	require.False(t, called)
	for _, id := range []string{"", "0123456789ABCDEFGHIJK!", "0123456789ABCDEFGHIJK"} {
		_, _, err := h.Services.ProposeTrack(context.Background(), uuid.New(), session.Session{}, id)
		requireProposeError(t, err, "INVALID_SPOTIFY_TRACK_ID", http.StatusBadRequest)
	}
}

func TestProposeTrackMissingFromSpotify(t *testing.T) {
	h, roomID, current, _ := proposeHarness(t, 20, 0)
	h.Repos.SpotifyRepo.GetBySpotifyIDFn = func(context.Context, string) (string, domainspotify.Track, error) {
		return "", domainspotify.Track{}, pgx.ErrNoRows
	}
	h.Repos.SpotifyProvider.GetTrackFn = func(context.Context, string) (domainspotify.Track, error) {
		return domainspotify.Track{}, &domainspotify.ProviderError{StatusCode: http.StatusNotFound}
	}
	_, _, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
	requireProposeError(t, err, "SPOTIFY_TRACK_NOT_FOUND", http.StatusNotFound)
}

func TestProposeTrackDuplicateBeforeQueueLimit(t *testing.T) {
	for _, count := range []int{1, 20} {
		h, roomID, current, _ := proposeHarness(t, 20, count)
		existingID := uuid.New()
		h.Repos.TrackRepo.GetActiveDuplicateFn = func(context.Context, uuid.UUID, uuid.UUID) (track.DuplicateInfo, error) {
			return track.DuplicateInfo{ID: existingID, CurrentVoteCount: 4}, nil
		}
		h.Repos.TrackRepo.CountQueuedFn = func(context.Context, uuid.UUID) (int, error) {
			t.Fatal("queue count must follow duplicate check")
			return 0, nil
		}
		response, status, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status)
		require.True(t, response.Duplicate)
		require.Equal(t, existingID, response.ExistingRoomTrack.ID)
		require.Empty(t, h.Broadcaster.Events)
	}
}

func TestProposeTrackRejectsNewTrackWhenQueueFull(t *testing.T) {
	h, roomID, current, _ := proposeHarness(t, 20, 20)
	_, _, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
	requireProposeError(t, err, "QUEUE_FULL", http.StatusConflict)
}

func TestProposeTrackPlayingDoesNotConsumeQueueSlotButRemainsDuplicate(t *testing.T) {
	roomTrackID := uuid.New()
	h, roomID, current, _ := proposeHarness(t, 2, 1) // one playing + one queued
	h.Repos.TrackRepo.CreateQueuedIfAbsentFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (track.RoomTrack, bool, error) {
		return track.RoomTrack{ID: roomTrackID, Status: track.StatusQueued}, true, nil
	}
	response, status, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, roomTrackID, response.RoomTrack.ID)

	h, roomID, current, _ = proposeHarness(t, 2, 2) // one playing + two queued
	_, _, err = h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
	requireProposeError(t, err, "QUEUE_FULL", http.StatusConflict)

	h.Repos.TrackRepo.GetActiveDuplicateFn = func(context.Context, uuid.UUID, uuid.UUID) (track.DuplicateInfo, error) {
		return track.DuplicateInfo{ID: roomTrackID}, nil // playing remains active for deduplication
	}
	response, status, err = h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.True(t, response.Duplicate)
	require.Equal(t, roomTrackID, response.ExistingRoomTrack.ID)
}

func TestProposeTrackBroadcastsOnlyAfterCommit(t *testing.T) {
	h, roomID, current, _ := proposeHarness(t, 2, 0)
	uow := &commitTrackingUOW{repos: h.Repos}
	h.Services.UOW = uow
	h.Repos.TrackRepo.CreateQueuedIfAbsentFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (track.RoomTrack, bool, error) {
		return track.RoomTrack{ID: uuid.New()}, true, nil
	}
	h.Broadcaster.BroadcastFn = func(event usecase.LiveEvent) {
		require.True(t, uow.committed)
		require.Equal(t, "track_proposed", event.Name)
	}
	_, status, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	require.Len(t, h.Broadcaster.Events, 1)
}

func TestProposeTrackRejectsSessionFromOtherRoom(t *testing.T) {
	h, roomID, current, _ := proposeHarness(t, 20, 0)
	current.RoomID = uuid.New()
	_, _, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
	requireProposeError(t, err, "SESSION_NOT_IN_ROOM", http.StatusForbidden)
}

func TestProposeTrackRequiresActiveRoom(t *testing.T) {
	for _, status := range []string{room.StatusDraft, room.StatusPaused, room.StatusClosed} {
		h, roomID, current, _ := proposeHarness(t, 20, 0)
		h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
			return room.Room{ID: roomID, Status: status, QueueLimit: 20}, nil
		}
		_, _, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
		requireProposeError(t, err, "ROOM_NOT_ACTIVE", http.StatusForbidden)
	}
}

func TestProposeTrackCreatedAndConcurrentConflictResult(t *testing.T) {
	for _, created := range []bool{true, false} {
		h, roomID, current, _ := proposeHarness(t, 20, 0)
		createdID := uuid.New()
		h.Repos.TrackRepo.CreateQueuedIfAbsentFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (track.RoomTrack, bool, error) {
			if created {
				return track.RoomTrack{ID: createdID}, true, nil
			}
			return track.RoomTrack{}, false, nil
		}
		duplicateLookups := 0
		h.Repos.TrackRepo.GetActiveDuplicateFn = func(context.Context, uuid.UUID, uuid.UUID) (track.DuplicateInfo, error) {
			duplicateLookups++
			if duplicateLookups == 1 {
				return track.DuplicateInfo{}, pgx.ErrNoRows
			}
			// The second lookup represents the row inserted by a concurrent request.
			return track.DuplicateInfo{ID: createdID}, nil
		}
		response, status, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
		require.NoError(t, err)
		if created {
			require.Equal(t, http.StatusCreated, status)
			require.False(t, response.Duplicate)
			require.Equal(t, createdID, response.RoomTrack.ID)
		} else {
			require.Equal(t, http.StatusOK, status)
			require.True(t, response.Duplicate)
			require.Equal(t, createdID, response.ExistingRoomTrack.ID)
			require.Equal(t, 2, duplicateLookups)
		}
	}
}
