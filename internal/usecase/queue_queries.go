package usecase

import (
	"context"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"

	"github.com/google/uuid"
)

func (s *Services) GetQueue(ctx context.Context, roomID uuid.UUID) (queue.Snapshot, error) {
	var snapshot queue.Snapshot
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		if _, err := repos.Rooms().GetByIDForShare(ctx, roomID); err != nil {
			return err
		}
		var err error
		snapshot, err = repos.Queue().LoadSnapshot(ctx, roomID)
		return err
	})
	return snapshot, err
}

func (s *Services) ListQueueRooms(ctx context.Context) ([]room.ScheduleEntry, error) {
	return s.Repos.Rooms().ListSchedulable(ctx)
}
