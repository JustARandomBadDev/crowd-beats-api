package queue

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	LoadSnapshot(ctx context.Context, roomID uuid.UUID) (Snapshot, error)
	LoadState(ctx context.Context, roomID uuid.UUID) (SnapshotState, error)
	LoadNowPlaying(ctx context.Context, roomID uuid.UUID) (*Item, error)
	ListRankedQueued(ctx context.Context, roomID uuid.UUID, limit int) ([]RankedItem, error)
	Replace(ctx context.Context, roomID uuid.UUID, items []RankedItem, recalculatedAt time.Time) error
}
