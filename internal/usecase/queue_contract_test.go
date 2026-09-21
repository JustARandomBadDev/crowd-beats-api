package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/testutil"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type commitTrackingUOW struct {
	repos     usecase.RepositorySet
	committed bool
}

func (u *commitTrackingUOW) Run(ctx context.Context, fn func(usecase.RepositorySet) error) error {
	err := fn(u.repos)
	if err == nil {
		u.committed = true
	}
	return err
}

func TestRecalculateQueueChangesOnlyOnFunctionalChangeAndBroadcastsAfterCommit(t *testing.T) {
	h := testutil.NewHarness()
	roomID, queuedID, playingID := uuid.New(), uuid.New(), uuid.New()
	uow := &commitTrackingUOW{repos: h.Repos}
	h.Services.UOW = uow
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, QueueLimit: 2}, nil
	}
	var state queue.SnapshotState
	h.Repos.QueueRepo.LoadStateFn = func(context.Context, uuid.UUID) (queue.SnapshotState, error) { return state, nil }
	ranked := []queue.RankedItem{{RoomTrackID: queuedID, VoteCount: 3, FIFOOrder: 1}}
	h.Repos.QueueRepo.ListRankedQueuedFn = func(context.Context, uuid.UUID, int) ([]queue.RankedItem, error) { return ranked, nil }
	var playing *queue.Item
	h.Repos.QueueRepo.LoadNowPlayingFn = func(context.Context, uuid.UUID) (*queue.Item, error) { return playing, nil }
	replacements, cacheUpdates := 0, 0
	h.Repos.QueueRepo.ReplaceFn = func(context.Context, uuid.UUID, []queue.RankedItem, time.Time) error {
		replacements++
		return nil
	}
	h.Repos.TrackRepo.UpdateCachedScoresFn = func(_ context.Context, _ uuid.UUID, scores map[uuid.UUID]int) error {
		cacheUpdates++
		require.Equal(t, 3, scores[queuedID])
		return nil
	}
	h.Repos.EventRepo.InsertFn = func(_ context.Context, _ uuid.UUID, event string, payload map[string]any) error {
		require.Equal(t, "queue_recalculated", event)
		state = queue.SnapshotState{Fingerprint: payload["fingerprint"].(string), UpdatedAt: ptrTime(payload["updated_at"].(time.Time))}
		return nil
	}
	h.Repos.QueueRepo.LoadSnapshotFn = func(context.Context, uuid.UUID) (queue.Snapshot, error) {
		return queue.Snapshot{Items: []queue.Item{{RoomTrackID: queuedID}}, NowPlaying: playing, UpdatedAt: state.UpdatedAt}, nil
	}
	h.Broadcaster.BroadcastFn = func(event usecase.LiveEvent) {
		require.True(t, uow.committed, "queue_updated must follow commit")
		require.Equal(t, "queue_updated", event.Name)
	}
	changed, err := h.Services.RecalculateRoomQueue(context.Background(), roomID)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, replacements)
	require.Equal(t, 1, cacheUpdates)
	require.Len(t, h.Broadcaster.Events, 1)

	uow.committed = false
	changed, err = h.Services.RecalculateRoomQueue(context.Background(), roomID)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, 1, replacements)
	require.Len(t, h.Broadcaster.Events, 1)

	playing = &queue.Item{RoomTrackID: playingID, VoteCount: 1}
	previousUpdatedAt := *state.UpdatedAt
	uow.committed = false
	changed, err = h.Services.RecalculateRoomQueue(context.Background(), roomID)
	require.NoError(t, err)
	require.True(t, changed, "now_playing is part of the public snapshot fingerprint")
	require.True(t, state.UpdatedAt.After(previousUpdatedAt), "snapshot versions must increase even when the clock has not advanced")
	require.Equal(t, 2, replacements)
	require.Len(t, h.Broadcaster.Events, 2)

	nickname := "Renamed guest"
	ranked[0].ProposedBy = &nickname
	uow.committed = false
	changed, err = h.Services.RecalculateRoomQueue(context.Background(), roomID)
	require.NoError(t, err)
	require.True(t, changed, "public proposed_by changes require a new snapshot")
	require.Equal(t, 3, replacements)
}

func ptrTime(value time.Time) *time.Time { return &value }

func TestFailedTransactionsDoNotBroadcastQueueProposalOrVote(t *testing.T) {
	t.Run("recalculation", func(t *testing.T) {
		h := testutil.NewHarness()
		h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
			return room.Room{QueueLimit: 2}, nil
		}
		h.Repos.EventRepo.InsertFn = func(context.Context, uuid.UUID, string, map[string]any) error {
			return errors.New("commit would roll back")
		}
		_, err := h.Services.RecalculateRoomQueue(context.Background(), uuid.New())
		require.Error(t, err)
		require.Empty(t, h.Broadcaster.Events)
	})
	t.Run("proposal", func(t *testing.T) {
		h, roomID, current, _ := proposeHarness(t, 2, 0)
		h.Repos.TrackRepo.CreateQueuedIfAbsentFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (track.RoomTrack, bool, error) {
			return track.RoomTrack{ID: uuid.New()}, true, nil
		}
		h.Repos.EventRepo.InsertFn = func(context.Context, uuid.UUID, string, map[string]any) error {
			return errors.New("commit would roll back")
		}
		_, _, err := h.Services.ProposeTrack(context.Background(), roomID, current, validTrackID)
		require.Error(t, err)
		require.Empty(t, h.Broadcaster.Events)
	})
	t.Run("vote", func(t *testing.T) {
		h := testutil.NewHarness()
		roomID, sessionID, trackID := uuid.New(), uuid.New(), uuid.New()
		h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
			return room.Room{ID: roomID, Status: room.StatusActive, MaxVotesPerUser: 5}, nil
		}
		h.Repos.SessionRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (session.Session, error) {
			return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
		}
		h.Repos.TrackRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (track.RoomTrack, error) {
			return track.RoomTrack{ID: trackID, RoomID: roomID, Status: track.StatusQueued}, nil
		}
		h.Repos.EventRepo.InsertFn = func(context.Context, uuid.UUID, string, map[string]any) error {
			return errors.New("commit would roll back")
		}
		_, err := h.Services.AddVote(context.Background(), roomID, session.Session{ID: sessionID}, trackID)
		require.Error(t, err)
		require.Empty(t, h.Broadcaster.Events)
	})
}

func TestVoteBroadcastExcludesPersonalQuotaAndFollowsCommit(t *testing.T) {
	h := testutil.NewHarness()
	roomID, sessionID, trackID := uuid.New(), uuid.New(), uuid.New()
	uow := &commitTrackingUOW{repos: h.Repos}
	h.Services.UOW = uow
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, Status: room.StatusActive, MaxVotesPerUser: 5}, nil
	}
	h.Repos.SessionRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (session.Session, error) {
		return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
	}
	h.Repos.TrackRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (track.RoomTrack, error) {
		return track.RoomTrack{ID: trackID, RoomID: roomID, Status: track.StatusQueued, VoteCountCached: 2}, nil
	}
	h.Broadcaster.BroadcastFn = func(event usecase.LiveEvent) {
		require.True(t, uow.committed)
		require.Equal(t, "vote_received", event.Name)
		payload := event.Payload.(map[string]any)
		require.NotContains(t, payload, "votes_remaining")
		require.Equal(t, 3, payload["current_vote_count"])
	}
	result, err := h.Services.AddVote(context.Background(), roomID, session.Session{ID: sessionID}, trackID)
	require.NoError(t, err)
	require.Equal(t, 4, result.VotesRemaining)
	require.Len(t, h.Broadcaster.Events, 1)
}
