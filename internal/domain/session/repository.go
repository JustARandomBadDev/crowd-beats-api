package session

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	GetByTokenHash(ctx context.Context, hash string) (Session, error)
	Create(ctx context.Context, input CreateInput) (Session, error)
	Reattach(ctx context.Context, sessionID uuid.UUID, roomID uuid.UUID, nickname string) error
	GetByIDForUpdate(ctx context.Context, sessionID uuid.UUID) (Session, error)
	Heartbeat(ctx context.Context, sessionID uuid.UUID) error
	Leave(ctx context.Context, sessionID uuid.UUID) error
}
