package queue

import (
	"time"

	"github.com/google/uuid"
)

type Item struct {
	Position       int
	RoomTrackID    uuid.UUID
	Score          int
	VoteCount      int
	FIFOOrder      int64
	SpotifyTrackID string
	Title          string
	ArtistNames    string
	AlbumName      string
	ImageURL       string
	PreviewURL     string
	URI            string
	ProposedBy     *string
	RecalculatedAt time.Time
}

type RankedItem struct {
	RoomTrackID uuid.UUID
	VoteCount   int
	FIFOOrder   int64
}
