package spotify

import "context"

type Provider interface {
	SearchTracks(ctx context.Context, query string) ([]Track, error)
	GetTrack(ctx context.Context, spotifyTrackID string) (Track, error)
}

type Repository interface {
	GetBySpotifyID(ctx context.Context, spotifyTrackID string) (string, Track, error)
	Upsert(ctx context.Context, track Track) (string, error)
	SearchLocal(ctx context.Context, query string) ([]Track, error)
}
