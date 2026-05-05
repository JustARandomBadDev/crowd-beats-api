package room

import (
	"time"

	"github.com/google/uuid"
)

type Room struct {
	ID                    uuid.UUID
	Name                  string
	Slug                  *string
	Status                string
	QueueLimit            int
	MaxVotesPerUser       int
	QRTTLSeconds          int
	RecalcIntervalSeconds int
	ManagerSecretHash     *string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type QRCode struct {
	Code      string
	ExpiresAt time.Time
}

type Patch struct {
	Status          *string
	QueueLimit      *int
	MaxVotesPerUser *int
}

type CreateInput struct {
	Name                  string
	Slug                  *string
	Status                string
	QueueLimit            int
	MaxVotesPerUser       int
	QRTTLSeconds          int
	RecalcIntervalSeconds int
	ManagerSecretHash     string
}

type TopTrack struct {
	Title       string
	ArtistNames string
	Votes       int
}

type Stats struct {
	ActiveUsers   int
	TracksInQueue int
	VotesCount    int
	TopTracks     []TopTrack
}
