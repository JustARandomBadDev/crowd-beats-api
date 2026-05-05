package usecase

import (
	"context"
	"database/sql"

	"crowdbeats/internal/domain/queue"

	"github.com/google/uuid"
)

func (s *Services) RecalculateRoomQueue(ctx context.Context, roomID uuid.UUID) (bool, error) {
	var changed bool
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByID(ctx, roomID)
		if err != nil {
			return err
		}
		oldItems, _, err := repos.Queue().Load(ctx, roomID)
		if err != nil {
			return err
		}
		ranked, err := repos.Queue().ListRankedQueued(ctx, roomID, targetRoom.QueueLimit)
		if err != nil {
			return err
		}
		if err := repos.Queue().Replace(ctx, roomID, ranked, s.Now()); err != nil {
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
			"count":      len(ranked),
			"updated_at": s.Now(),
		}); err != nil {
			return err
		}
		newItems, updatedAt, err := repos.Queue().Load(ctx, roomID)
		if err != nil {
			return err
		}
		s.DirtyRooms.Clear(roomID)
		changed = queueChanged(oldItems, newItems)
		if changed {
			s.Broadcaster.Broadcast(LiveEvent{
				Name:      "queue_updated",
				RoomID:    roomID,
				Timestamp: s.Now(),
				Payload: map[string]any{
					"updated_at": updatedAt,
					"items":      newItems,
				},
			})
		}
		return nil
	})
	return changed, err
}

func (s *Services) RecalculateDirtyRooms(ctx context.Context) {
	for _, roomID := range s.DirtyRooms.List() {
		_, _ = s.RecalculateRoomQueue(ctx, roomID)
	}
}

func queueChanged(before, after []queue.Item) bool {
	if len(before) != len(after) {
		return true
	}
	for i := range before {
		if before[i].RoomTrackID != after[i].RoomTrackID ||
			before[i].Position != after[i].Position ||
			before[i].Score != after[i].Score ||
			before[i].VoteCount != after[i].VoteCount {
			return true
		}
	}
	return false
}

func nullableInt(v sql.NullInt32) *int {
	if !v.Valid {
		return nil
	}
	value := int(v.Int32)
	return &value
}
