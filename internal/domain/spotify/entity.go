package spotify

import "encoding/json"

type Track struct {
	SpotifyTrackID string
	Title          string
	ArtistNames    string
	AlbumName      string
	DurationMS     int
	ImageURL       string
	PreviewURL     string
	URI            string
	RawPayload     json.RawMessage
}
