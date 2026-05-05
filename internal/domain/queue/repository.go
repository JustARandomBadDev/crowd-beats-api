package queue

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	Load(ctx context.Context, roomID uuid.UUID) ([]Item, *time.Time, error)
	ListRankedQueued(ctx context.Context, roomID uuid.UUID, limit int) ([]RankedItem, error)
	Replace(ctx context.Context, roomID uuid.UUID, items []RankedItem, recalculatedAt time.Time) error
}
