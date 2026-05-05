package usecase

import (
	"context"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/spotify"
	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/domain/vote"

	"github.com/google/uuid"
)

type EventRepository interface {
	Insert(ctx context.Context, roomID uuid.UUID, eventType string, payload map[string]any) error
}

type RepositorySet interface {
	Rooms() room.Repository
	Sessions() session.Repository
	Spotify() spotify.Repository
	Tracks() track.Repository
	Queue() queue.Repository
	Votes() vote.Repository
	Events() EventRepository
}

type UnitOfWork interface {
	Run(ctx context.Context, fn func(repos RepositorySet) error) error
}

type Broadcaster interface {
	Broadcast(event LiveEvent)
}

type LiveEvent struct {
	Name      string
	RoomID    uuid.UUID
	Timestamp time.Time
	Payload   any
}

type DirtyTracker interface {
	Mark(roomID uuid.UUID)
	Clear(roomID uuid.UUID)
	List() []uuid.UUID
}

type SearchCache interface {
	Get(key string) ([]spotify.Track, bool)
	Set(key string, items []spotify.Track)
}

type TokenManager interface {
	NewToken(bytes int) (string, error)
	Hash(token string) string
}

type Services struct {
	UOW             UnitOfWork
	Repos           RepositorySet
	SpotifyProvider spotify.Provider
	Broadcaster     Broadcaster
	DirtyRooms      DirtyTracker
	SearchCache     SearchCache
	Tokens          TokenManager
	Now             func() time.Time
}

func NewServices(
	uow UnitOfWork,
	repos RepositorySet,
	spotifyProvider spotify.Provider,
	broadcaster Broadcaster,
	dirty DirtyTracker,
	searchCache SearchCache,
	tokens TokenManager,
) *Services {
	return &Services{
		UOW:             uow,
		Repos:           repos,
		SpotifyProvider: spotifyProvider,
		Broadcaster:     broadcaster,
		DirtyRooms:      dirty,
		SearchCache:     searchCache,
		Tokens:          tokens,
		Now: func() time.Time {
			return time.Now().UTC()
		},
	}
}
