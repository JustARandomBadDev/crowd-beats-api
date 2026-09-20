package vote

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CountBySessionInRoom(ctx context.Context, roomID, sessionID uuid.UUID) (int, error)
	HasBySessionAndTrack(ctx context.Context, sessionID, roomTrackID uuid.UUID) (bool, error)
	Insert(ctx context.Context, roomID, roomTrackID, sessionID uuid.UUID) error
}
