package track

import (
	"time"

	"github.com/google/uuid"
)

type RoomTrack struct {
	ID                  uuid.UUID
	RoomID              uuid.UUID
	SpotifyTrackRefID   uuid.UUID
	ProposedBySessionID *uuid.UUID
	Status              string
	VoteCountCached     int
	ScoreCached         int
	FIFOOrder           int64
	ProposedAt          time.Time
}

type DuplicateInfo struct {
	ID               uuid.UUID
	CurrentVoteCount int
	Position         *int
}
