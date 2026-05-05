package vote

import (
	"time"

	"github.com/google/uuid"
)

type Vote struct {
	RoomID      uuid.UUID
	RoomTrackID uuid.UUID
	SessionID   uuid.UUID
	CreatedAt   time.Time
}
