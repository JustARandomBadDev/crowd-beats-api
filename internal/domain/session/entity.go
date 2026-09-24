package session

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID               uuid.UUID
	RoomID           uuid.UUID
	Nickname         string
	Role             string
	Status           string
	SessionTokenHash string
	LastSeenAt       time.Time
}

type CreateInput struct {
	RoomID           uuid.UUID
	SessionTokenHash string
	Nickname         string
	Role             string
}
