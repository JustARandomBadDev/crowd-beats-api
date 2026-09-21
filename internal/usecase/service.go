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
	DisconnectSession(roomID, sessionID uuid.UUID)
}

type LiveEvent struct {
	Name      string
	RoomID    uuid.UUID
	Timestamp time.Time
	Payload   any
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
	SearchCache     SearchCache
	Tokens          TokenManager
	Now             func() time.Time
}

func NewServices(
	uow UnitOfWork,
	repos RepositorySet,
	spotifyProvider spotify.Provider,
	broadcaster Broadcaster,
	searchCache SearchCache,
	tokens TokenManager,
) *Services {
	return &Services{
		UOW:             uow,
		Repos:           repos,
		SpotifyProvider: spotifyProvider,
		Broadcaster:     broadcaster,
		SearchCache:     searchCache,
		Tokens:          tokens,
		Now: func() time.Time {
			return time.Now().UTC()
		},
	}
}
