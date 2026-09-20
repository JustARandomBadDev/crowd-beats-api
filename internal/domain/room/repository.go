package room

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	Create(ctx context.Context, input CreateInput) (Room, error)
	GetByID(ctx context.Context, roomID uuid.UUID) (Room, error)
	GetByIDForUpdate(ctx context.Context, roomID uuid.UUID) (Room, error)
	GetByQRCode(ctx context.Context, code string) (Room, error)
	RotateQRCode(ctx context.Context, roomID uuid.UUID, code string, expiresAt time.Time) error
	Patch(ctx context.Context, roomID uuid.UUID, patch Patch) (Room, error)
	LoadStats(ctx context.Context, roomID uuid.UUID) (Stats, error)
	CountActiveUsers(ctx context.Context, roomID uuid.UUID) (int, error)
}
