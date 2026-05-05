package ws

import (
	"time"

	"github.com/google/uuid"
)

type Event struct {
	Event     string    `json:"event"`
	RoomID    uuid.UUID `json:"room_id"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}
