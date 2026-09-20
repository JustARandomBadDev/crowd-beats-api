package usecase

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"crowdbeats/internal/domain/spotify"
	"crowdbeats/pkg/apierror"
)

func (s *Services) SearchSpotify(ctx context.Context, query string) ([]spotify.Track, error) {
	key := "search:" + strings.ToLower(strings.TrimSpace(query))
	if items, ok := s.SearchCache.Get(key); ok {
		return items, nil
	}
	items, err := s.SpotifyProvider.SearchTracks(ctx, query)
	if err != nil {
		return nil, publicSpotifyError(err)
	}
	s.SearchCache.Set(key, items)
	return items, nil
}

func publicSpotifyError(err error) error {
	var providerErr *spotify.ProviderError
	if !errors.As(err, &providerErr) {
		return err
	}
	if providerErr.StatusCode == http.StatusTooManyRequests {
		return apierror.New("SPOTIFY_RATE_LIMITED", "Spotify is temporarily rate limited", http.StatusServiceUnavailable)
	}
	return apierror.New("SPOTIFY_UNAVAILABLE", "Spotify is temporarily unavailable", http.StatusServiceUnavailable)
}

func publicSpotifyTrackError(err error) error {
	var providerErr *spotify.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.StatusCode == http.StatusNotFound {
			return apierror.New("SPOTIFY_TRACK_NOT_FOUND", "Spotify track not found", http.StatusNotFound)
		}
		return publicSpotifyError(err)
	}
	return apierror.New("SPOTIFY_UNAVAILABLE", "Spotify is temporarily unavailable", http.StatusServiceUnavailable)
}
