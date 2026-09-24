package usecase_test

import (
	"context"
	"errors"
	"testing"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/testutil"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestMarkPlayingLocksRoomAndBroadcastsAfterCommit(t *testing.T) {
	h := testutil.NewHarness()
	roomID, trackID := uuid.New(), uuid.New()
	uow := &commitTrackingUOW{repos: h.Repos}
	h.Services.UOW = uow
	locked := false
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
		locked = true
		return room.Room{ID: roomID}, nil
	}
	h.Repos.TrackRepo.SetPlayingFn = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
		require.True(t, locked, "room must be locked before changing playing track")
		return true, nil
	}
	h.Broadcaster.BroadcastFn = func(event usecase.LiveEvent) {
		require.True(t, uow.committed)
		require.Equal(t, "track_playing", event.Name)
	}
	require.NoError(t, h.Services.SetTrackPlaying(context.Background(), roomID, trackID))
	require.Len(t, h.Broadcaster.Events, 1)

	h = testutil.NewHarness()
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID}, nil
	}
	h.Repos.TrackRepo.SetPlayingFn = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
		return true, nil
	}
	h.Repos.EventRepo.InsertFn = func(context.Context, uuid.UUID, string, map[string]any) error {
		return errors.New("transaction failed")
	}
	require.Error(t, h.Services.SetTrackPlaying(context.Background(), roomID, trackID))
	require.Empty(t, h.Broadcaster.Events)
}

func TestTrackLifecycleDoesNotSynchronouslyRecalculateAfterCommit(t *testing.T) {
	roomID, trackID := uuid.New(), uuid.New()
	for _, flow := range []struct {
		name, event string
		run         func(*testutil.Harness) error
	}{
		{"delete", "track_deleted", func(h *testutil.Harness) error { return h.Services.DeleteTrack(context.Background(), roomID, trackID) }},
		{"skip", "track_skipped", func(h *testutil.Harness) error {
			return h.Services.SetTrackStatus(context.Background(), roomID, trackID, "skipped", "track_skipped", "skipped_at")
		}},
		{"played", "track_played", func(h *testutil.Harness) error {
			return h.Services.SetTrackStatus(context.Background(), roomID, trackID, "played", "track_played", "played_at")
		}},
	} {
		t.Run(flow.name, func(t *testing.T) {
			h := testutil.NewHarness()
			h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
				return room.Room{ID: roomID}, nil
			}
			h.Repos.TrackRepo.DeleteFn = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
			h.Repos.TrackRepo.SetStatusFn = func(context.Context, uuid.UUID, uuid.UUID, string, string) (bool, error) { return true, nil }
			h.Repos.QueueRepo.ListRankedQueuedFn = func(context.Context, uuid.UUID, int) ([]queue.RankedItem, error) {
				t.Fatal("manager action must leave recalculation to scheduler")
				return nil, nil
			}
			require.NoError(t, flow.run(h))
			require.Len(t, h.Broadcaster.Events, 1)
			require.Equal(t, flow.event, h.Broadcaster.Events[0].Name)
		})
	}
}
