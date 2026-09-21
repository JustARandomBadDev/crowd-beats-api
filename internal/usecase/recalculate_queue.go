package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"crowdbeats/internal/domain/queue"

	"github.com/google/uuid"
)

func (s *Services) RecalculateRoomQueue(ctx context.Context, roomID uuid.UUID) (bool, error) {
	var changed bool
	var snapshot queue.Snapshot
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByIDForUpdate(ctx, roomID)
		if err != nil {
			return err
		}
		state, err := repos.Queue().LoadState(ctx, roomID)
		if err != nil {
			return err
		}
		ranked, err := repos.Queue().ListRankedQueued(ctx, roomID, targetRoom.QueueLimit)
		if err != nil {
			return err
		}
		playing, err := repos.Queue().LoadNowPlaying(ctx, roomID)
		if err != nil {
			return err
		}
		fingerprint, err := queueFingerprint(ranked, playing)
		if err != nil {
			return err
		}
		changed = state.UpdatedAt == nil || state.Fingerprint != fingerprint
		if !changed {
			return nil
		}
		updatedAt := s.Now()
		if state.UpdatedAt != nil && !updatedAt.After(*state.UpdatedAt) {
			updatedAt = state.UpdatedAt.Add(time.Microsecond)
		}
		if err := repos.Queue().Replace(ctx, roomID, ranked, updatedAt); err != nil {
			return err
		}
		scoreMap := make(map[uuid.UUID]int, len(ranked))
		for _, item := range ranked {
			scoreMap[item.RoomTrackID] = item.VoteCount
		}
		if err := repos.Tracks().UpdateCachedScores(ctx, roomID, scoreMap); err != nil {
			return err
		}
		if err := repos.Events().Insert(ctx, roomID, "queue_recalculated", map[string]any{
			"count": len(ranked), "updated_at": updatedAt, "fingerprint": fingerprint,
		}); err != nil {
			return err
		}
		snapshot, err = repos.Queue().LoadSnapshot(ctx, roomID)
		return err
	})
	if err != nil {
		return false, err
	}
	if changed {
		s.Broadcaster.Broadcast(LiveEvent{
			Name: "queue_updated", RoomID: roomID, Timestamp: s.Now(), Payload: snapshot,
		})
	}
	return changed, nil
}

func queueFingerprint(items []queue.RankedItem, playing *queue.Item) (string, error) {
	type rankedFingerprint struct {
		ID         uuid.UUID `json:"id"`
		VoteCount  int       `json:"vote_count"`
		ProposedBy *string   `json:"proposed_by"`
	}
	type playingFingerprint struct {
		ID         uuid.UUID `json:"id"`
		VoteCount  int       `json:"vote_count"`
		ProposedBy *string   `json:"proposed_by"`
	}
	version := struct {
		Items      []rankedFingerprint `json:"items"`
		NowPlaying *playingFingerprint `json:"now_playing"`
	}{Items: make([]rankedFingerprint, 0, len(items))}
	for _, item := range items {
		version.Items = append(version.Items, rankedFingerprint{ID: item.RoomTrackID, VoteCount: item.VoteCount, ProposedBy: item.ProposedBy})
	}
	if playing != nil {
		version.NowPlaying = &playingFingerprint{ID: playing.RoomTrackID, VoteCount: playing.VoteCount, ProposedBy: playing.ProposedBy}
	}
	encoded, err := json.Marshal(version)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
