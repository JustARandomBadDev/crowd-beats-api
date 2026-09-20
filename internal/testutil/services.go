package testutil

import (
	"context"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	domainspotify "crowdbeats/internal/domain/spotify"
	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/domain/vote"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Harness struct {
	Repos       *FakeRepos
	Broadcaster *Broadcaster
	Dirty       *DirtyTracker
	Tokens      *TokenManager
	Services    *usecase.Services
}

func NewHarness() *Harness {
	repos := NewFakeRepos()
	broadcaster := &Broadcaster{}
	dirty := &DirtyTracker{}
	tokens := &TokenManager{
		NewTokenValue: "test-token",
	}
	services := usecase.NewServices(
		&UnitOfWork{Repos: repos},
		repos,
		repos.SpotifyProvider,
		broadcaster,
		dirty,
		&SearchCache{},
		tokens,
	)
	services.Now = func() time.Time { return time.Date(2026, 4, 1, 18, 0, 0, 0, time.UTC) }
	return &Harness{
		Repos:       repos,
		Broadcaster: broadcaster,
		Dirty:       dirty,
		Tokens:      tokens,
		Services:    services,
	}
}

type FakeRepos struct {
	RoomRepo        *RoomRepo
	SessionRepo     *SessionRepo
	SpotifyRepo     *SpotifyRepo
	SpotifyProvider *SpotifyProvider
	TrackRepo       *TrackRepo
	QueueRepo       *QueueRepo
	VoteRepo        *VoteRepo
	EventRepo       *EventRepo
}

func NewFakeRepos() *FakeRepos {
	return &FakeRepos{
		RoomRepo:        &RoomRepo{},
		SessionRepo:     &SessionRepo{},
		SpotifyRepo:     &SpotifyRepo{},
		SpotifyProvider: &SpotifyProvider{},
		TrackRepo:       &TrackRepo{},
		QueueRepo:       &QueueRepo{},
		VoteRepo:        &VoteRepo{},
		EventRepo:       &EventRepo{},
	}
}

func (f *FakeRepos) Rooms() room.Repository            { return f.RoomRepo }
func (f *FakeRepos) Sessions() session.Repository      { return f.SessionRepo }
func (f *FakeRepos) Spotify() domainspotify.Repository { return f.SpotifyRepo }
func (f *FakeRepos) Tracks() track.Repository          { return f.TrackRepo }
func (f *FakeRepos) Queue() queue.Repository           { return f.QueueRepo }
func (f *FakeRepos) Votes() vote.Repository            { return f.VoteRepo }
func (f *FakeRepos) Events() usecase.EventRepository   { return f.EventRepo }

type UnitOfWork struct {
	Repos usecase.RepositorySet
}

func (u *UnitOfWork) Run(ctx context.Context, fn func(repos usecase.RepositorySet) error) error {
	return fn(u.Repos)
}

type Broadcaster struct {
	Events []usecase.LiveEvent
}

func (b *Broadcaster) Broadcast(event usecase.LiveEvent) {
	b.Events = append(b.Events, event)
}

type DirtyTracker struct {
	Marked []uuid.UUID
}

func (d *DirtyTracker) Mark(roomID uuid.UUID) {
	d.Marked = append(d.Marked, roomID)
}

func (d *DirtyTracker) Clear(roomID uuid.UUID) {}

func (d *DirtyTracker) List() []uuid.UUID { return nil }

type SearchCache struct {
	Items map[string][]domainspotify.Track
}

func (c *SearchCache) Get(key string) ([]domainspotify.Track, bool) {
	if c.Items == nil {
		return nil, false
	}
	items, ok := c.Items[key]
	return items, ok
}

func (c *SearchCache) Set(key string, items []domainspotify.Track) {
	if c.Items == nil {
		c.Items = map[string][]domainspotify.Track{}
	}
	c.Items[key] = items
}

type TokenManager struct {
	NewTokenValue string
	NewTokenFn    func(int) (string, error)
	HashFn        func(string) string
}

func (t *TokenManager) NewToken(size int) (string, error) {
	if t.NewTokenFn != nil {
		return t.NewTokenFn(size)
	}
	return t.NewTokenValue, nil
}

func (t *TokenManager) Hash(value string) string {
	if t.HashFn != nil {
		return t.HashFn(value)
	}
	return "hashed:" + value
}

type RoomRepo struct {
	CreateFn           func(context.Context, room.CreateInput) (room.Room, error)
	GetByIDFn          func(context.Context, uuid.UUID) (room.Room, error)
	GetByIDForUpdateFn func(context.Context, uuid.UUID) (room.Room, error)
	GetByQRCodeFn      func(context.Context, string) (room.Room, error)
	RotateQRCodeFn     func(context.Context, uuid.UUID, string, time.Time) error
	PatchFn            func(context.Context, uuid.UUID, room.Patch) (room.Room, error)
	LoadStatsFn        func(context.Context, uuid.UUID) (room.Stats, error)
	CountActiveUsersFn func(context.Context, uuid.UUID) (int, error)
}

func (r *RoomRepo) Create(ctx context.Context, input room.CreateInput) (room.Room, error) {
	if r.CreateFn == nil {
		return room.Room{}, nil
	}
	return r.CreateFn(ctx, input)
}

func (r *RoomRepo) GetByID(ctx context.Context, roomID uuid.UUID) (room.Room, error) {
	if r.GetByIDFn == nil {
		return room.Room{}, nil
	}
	return r.GetByIDFn(ctx, roomID)
}

func (r *RoomRepo) GetByIDForUpdate(ctx context.Context, roomID uuid.UUID) (room.Room, error) {
	if r.GetByIDForUpdateFn != nil {
		return r.GetByIDForUpdateFn(ctx, roomID)
	}
	return r.GetByID(ctx, roomID)
}

func (r *RoomRepo) GetByQRCode(ctx context.Context, code string) (room.Room, error) {
	if r.GetByQRCodeFn == nil {
		return room.Room{}, nil
	}
	return r.GetByQRCodeFn(ctx, code)
}

func (r *RoomRepo) RotateQRCode(ctx context.Context, roomID uuid.UUID, code string, expiresAt time.Time) error {
	if r.RotateQRCodeFn == nil {
		return nil
	}
	return r.RotateQRCodeFn(ctx, roomID, code, expiresAt)
}

func (r *RoomRepo) Patch(ctx context.Context, roomID uuid.UUID, patch room.Patch) (room.Room, error) {
	if r.PatchFn == nil {
		return room.Room{}, nil
	}
	return r.PatchFn(ctx, roomID, patch)
}

func (r *RoomRepo) LoadStats(ctx context.Context, roomID uuid.UUID) (room.Stats, error) {
	if r.LoadStatsFn == nil {
		return room.Stats{}, nil
	}
	return r.LoadStatsFn(ctx, roomID)
}

func (r *RoomRepo) CountActiveUsers(ctx context.Context, roomID uuid.UUID) (int, error) {
	if r.CountActiveUsersFn == nil {
		return 0, nil
	}
	return r.CountActiveUsersFn(ctx, roomID)
}

type SessionRepo struct {
	GetByTokenHashFn          func(context.Context, string) (session.Session, error)
	GetByTokenHashForUpdateFn func(context.Context, string) (session.Session, error)
	CreateFn                  func(context.Context, session.CreateInput) (session.Session, error)
	ReattachSameRoomFn        func(context.Context, uuid.UUID, uuid.UUID, string) error
	GetByIDForUpdateFn        func(context.Context, uuid.UUID) (session.Session, error)
	HeartbeatFn               func(context.Context, uuid.UUID) error
	LeaveFn                   func(context.Context, uuid.UUID) error
}

func (r *SessionRepo) GetByTokenHash(ctx context.Context, hash string) (session.Session, error) {
	if r.GetByTokenHashFn == nil {
		return session.Session{}, nil
	}
	return r.GetByTokenHashFn(ctx, hash)
}

func (r *SessionRepo) GetByTokenHashForUpdate(ctx context.Context, hash string) (session.Session, error) {
	if r.GetByTokenHashForUpdateFn != nil {
		return r.GetByTokenHashForUpdateFn(ctx, hash)
	}
	return r.GetByTokenHash(ctx, hash)
}

func (r *SessionRepo) Create(ctx context.Context, input session.CreateInput) (session.Session, error) {
	if r.CreateFn == nil {
		return session.Session{}, nil
	}
	return r.CreateFn(ctx, input)
}

func (r *SessionRepo) ReattachSameRoom(ctx context.Context, sessionID uuid.UUID, roomID uuid.UUID, nickname string) error {
	if r.ReattachSameRoomFn == nil {
		return nil
	}
	return r.ReattachSameRoomFn(ctx, sessionID, roomID, nickname)
}

func (r *SessionRepo) GetByIDForUpdate(ctx context.Context, sessionID uuid.UUID) (session.Session, error) {
	if r.GetByIDForUpdateFn == nil {
		return session.Session{}, nil
	}
	return r.GetByIDForUpdateFn(ctx, sessionID)
}

func (r *SessionRepo) Heartbeat(ctx context.Context, sessionID uuid.UUID) error {
	if r.HeartbeatFn == nil {
		return nil
	}
	return r.HeartbeatFn(ctx, sessionID)
}

func (r *SessionRepo) Leave(ctx context.Context, sessionID uuid.UUID) error {
	if r.LeaveFn == nil {
		return nil
	}
	return r.LeaveFn(ctx, sessionID)
}

type SpotifyRepo struct {
	GetBySpotifyIDFn func(context.Context, string) (string, domainspotify.Track, error)
	UpsertFn         func(context.Context, domainspotify.Track) (string, error)
	SearchLocalFn    func(context.Context, string) ([]domainspotify.Track, error)
}

func (r *SpotifyRepo) GetBySpotifyID(ctx context.Context, spotifyTrackID string) (string, domainspotify.Track, error) {
	if r.GetBySpotifyIDFn == nil {
		return "", domainspotify.Track{}, nil
	}
	return r.GetBySpotifyIDFn(ctx, spotifyTrackID)
}

func (r *SpotifyRepo) Upsert(ctx context.Context, track domainspotify.Track) (string, error) {
	if r.UpsertFn == nil {
		return "", nil
	}
	return r.UpsertFn(ctx, track)
}

func (r *SpotifyRepo) SearchLocal(ctx context.Context, query string) ([]domainspotify.Track, error) {
	if r.SearchLocalFn == nil {
		return nil, nil
	}
	return r.SearchLocalFn(ctx, query)
}

type SpotifyProvider struct {
	SearchTracksFn func(context.Context, string) ([]domainspotify.Track, error)
	GetTrackFn     func(context.Context, string) (domainspotify.Track, error)
}

func (p *SpotifyProvider) SearchTracks(ctx context.Context, query string) ([]domainspotify.Track, error) {
	if p.SearchTracksFn == nil {
		return nil, nil
	}
	return p.SearchTracksFn(ctx, query)
}

func (p *SpotifyProvider) GetTrack(ctx context.Context, spotifyTrackID string) (domainspotify.Track, error) {
	if p.GetTrackFn == nil {
		return domainspotify.Track{}, nil
	}
	return p.GetTrackFn(ctx, spotifyTrackID)
}

type TrackRepo struct {
	CountActiveFn          func(context.Context, uuid.UUID) (int, error)
	CreateQueuedFn         func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (track.RoomTrack, error)
	CreateQueuedIfAbsentFn func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (track.RoomTrack, bool, error)
	GetActiveDuplicateFn   func(context.Context, uuid.UUID, uuid.UUID) (track.DuplicateInfo, error)
	GetByIDForUpdateFn     func(context.Context, uuid.UUID) (track.RoomTrack, error)
	DeleteFn               func(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	SetStatusFn            func(context.Context, uuid.UUID, uuid.UUID, string, string) (bool, error)
	SetPlayingFn           func(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	UpdateCachedScoresFn   func(context.Context, uuid.UUID, map[uuid.UUID]int) error
}

func (r *TrackRepo) CountActive(ctx context.Context, roomID uuid.UUID) (int, error) {
	if r.CountActiveFn == nil {
		return 0, nil
	}
	return r.CountActiveFn(ctx, roomID)
}

func (r *TrackRepo) CreateQueued(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID, sessionID uuid.UUID) (track.RoomTrack, error) {
	if r.CreateQueuedFn == nil {
		return track.RoomTrack{}, nil
	}
	return r.CreateQueuedFn(ctx, roomID, spotifyTrackRefID, sessionID)
}

func (r *TrackRepo) CreateQueuedIfAbsent(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID, sessionID uuid.UUID) (track.RoomTrack, bool, error) {
	if r.CreateQueuedIfAbsentFn != nil {
		return r.CreateQueuedIfAbsentFn(ctx, roomID, spotifyTrackRefID, sessionID)
	}
	created, err := r.CreateQueued(ctx, roomID, spotifyTrackRefID, sessionID)
	return created, err == nil, err
}

func (r *TrackRepo) GetActiveDuplicate(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID) (track.DuplicateInfo, error) {
	if r.GetActiveDuplicateFn == nil {
		return track.DuplicateInfo{}, pgx.ErrNoRows
	}
	return r.GetActiveDuplicateFn(ctx, roomID, spotifyTrackRefID)
}

func (r *TrackRepo) GetByIDForUpdate(ctx context.Context, roomTrackID uuid.UUID) (track.RoomTrack, error) {
	if r.GetByIDForUpdateFn == nil {
		return track.RoomTrack{}, nil
	}
	return r.GetByIDForUpdateFn(ctx, roomTrackID)
}

func (r *TrackRepo) Delete(ctx context.Context, roomID, roomTrackID uuid.UUID) (bool, error) {
	if r.DeleteFn == nil {
		return false, nil
	}
	return r.DeleteFn(ctx, roomID, roomTrackID)
}

func (r *TrackRepo) SetStatus(ctx context.Context, roomID, roomTrackID uuid.UUID, status string, timestampColumn string) (bool, error) {
	if r.SetStatusFn == nil {
		return false, nil
	}
	return r.SetStatusFn(ctx, roomID, roomTrackID, status, timestampColumn)
}

func (r *TrackRepo) SetPlaying(ctx context.Context, roomID, roomTrackID uuid.UUID) (bool, error) {
	if r.SetPlayingFn == nil {
		return false, nil
	}
	return r.SetPlayingFn(ctx, roomID, roomTrackID)
}

func (r *TrackRepo) UpdateCachedScores(ctx context.Context, roomID uuid.UUID, ranked map[uuid.UUID]int) error {
	if r.UpdateCachedScoresFn == nil {
		return nil
	}
	return r.UpdateCachedScoresFn(ctx, roomID, ranked)
}

type QueueRepo struct {
	LoadFn             func(context.Context, uuid.UUID) ([]queue.Item, *time.Time, error)
	ListRankedQueuedFn func(context.Context, uuid.UUID, int) ([]queue.RankedItem, error)
	ReplaceFn          func(context.Context, uuid.UUID, []queue.RankedItem, time.Time) error
}

func (r *QueueRepo) Load(ctx context.Context, roomID uuid.UUID) ([]queue.Item, *time.Time, error) {
	if r.LoadFn == nil {
		return nil, nil, nil
	}
	return r.LoadFn(ctx, roomID)
}

func (r *QueueRepo) ListRankedQueued(ctx context.Context, roomID uuid.UUID, limit int) ([]queue.RankedItem, error) {
	if r.ListRankedQueuedFn == nil {
		return nil, nil
	}
	return r.ListRankedQueuedFn(ctx, roomID, limit)
}

func (r *QueueRepo) Replace(ctx context.Context, roomID uuid.UUID, items []queue.RankedItem, recalculatedAt time.Time) error {
	if r.ReplaceFn == nil {
		return nil
	}
	return r.ReplaceFn(ctx, roomID, items, recalculatedAt)
}

type VoteRepo struct {
	CountBySessionInRoomFn func(context.Context, uuid.UUID, uuid.UUID) (int, error)
	HasBySessionAndTrackFn func(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	InsertFn               func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
}

func (r *VoteRepo) CountBySessionInRoom(ctx context.Context, roomID, sessionID uuid.UUID) (int, error) {
	if r.CountBySessionInRoomFn == nil {
		return 0, nil
	}
	return r.CountBySessionInRoomFn(ctx, roomID, sessionID)
}

func (r *VoteRepo) HasBySessionAndTrack(ctx context.Context, sessionID, roomTrackID uuid.UUID) (bool, error) {
	if r.HasBySessionAndTrackFn == nil {
		return false, nil
	}
	return r.HasBySessionAndTrackFn(ctx, sessionID, roomTrackID)
}

func (r *VoteRepo) Insert(ctx context.Context, roomID, roomTrackID, sessionID uuid.UUID) error {
	if r.InsertFn == nil {
		return nil
	}
	return r.InsertFn(ctx, roomID, roomTrackID, sessionID)
}

type EventRepo struct {
	InsertFn func(context.Context, uuid.UUID, string, map[string]any) error
}

func (r *EventRepo) Insert(ctx context.Context, roomID uuid.UUID, eventType string, payload map[string]any) error {
	if r.InsertFn == nil {
		return nil
	}
	return r.InsertFn(ctx, roomID, eventType, payload)
}
