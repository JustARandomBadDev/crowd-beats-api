package track

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CountQueued(ctx context.Context, roomID uuid.UUID) (int, error)
	CreateQueued(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID, sessionID uuid.UUID) (RoomTrack, error)
	CreateQueuedIfAbsent(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID, sessionID uuid.UUID) (RoomTrack, bool, error)
	GetActiveDuplicate(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID) (DuplicateInfo, error)
	GetByIDForUpdate(ctx context.Context, roomTrackID uuid.UUID) (RoomTrack, error)
	Delete(ctx context.Context, roomID, roomTrackID uuid.UUID) (bool, error)
	SetStatus(ctx context.Context, roomID, roomTrackID uuid.UUID, status string, timestampColumn string) (bool, error)
	SetPlaying(ctx context.Context, roomID, roomTrackID uuid.UUID) (bool, error)
	UpdateCachedScores(ctx context.Context, roomID uuid.UUID, ranked map[uuid.UUID]int) error
}
