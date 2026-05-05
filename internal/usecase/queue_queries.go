package usecase

import (
	"context"
	"time"

	"crowdbeats/internal/domain/queue"

	"github.com/google/uuid"
)

func (s *Services) GetQueue(ctx context.Context, roomID uuid.UUID) ([]queue.Item, *time.Time, error) {
	return s.Repos.Queue().Load(ctx, roomID)
}
