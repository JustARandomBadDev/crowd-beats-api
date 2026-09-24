package spotify

import "fmt"

// ProviderError identifies failures returned by the external Spotify service.
type ProviderError struct {
	StatusCode int
	Cause      error
}

func (e *ProviderError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("spotify request failed (status %d): %v", e.StatusCode, e.Cause)
	}
	return fmt.Sprintf("spotify request failed (status %d)", e.StatusCode)
}
