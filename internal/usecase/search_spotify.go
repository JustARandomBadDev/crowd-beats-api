package usecase

import (
	"context"
	"strings"

	"crowdbeats/internal/domain/spotify"
)

func (s *Services) SearchSpotify(ctx context.Context, query string) ([]spotify.Track, error) {
	key := "search:" + strings.ToLower(strings.TrimSpace(query))
	if items, ok := s.SearchCache.Get(key); ok {
		return items, nil
	}
	items, err := s.SpotifyProvider.SearchTracks(ctx, query)
	if err != nil {
		return nil, err
	}
	s.SearchCache.Set(key, items)
	return items, nil
}
