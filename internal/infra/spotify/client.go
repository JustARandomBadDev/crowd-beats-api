package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	domainspotify "crowdbeats/internal/domain/spotify"
)

type Config struct {
	ClientID string
	Secret   string
	TokenURL string
	APIBase  string
}

type Client struct {
	cfg        Config
	httpClient *http.Client
	local      domainspotify.Repository
	mu         sync.Mutex
	token      string
	expiresAt  time.Time
}

func NewClient(cfg Config, local domainspotify.Repository) *Client {
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		local:      local,
	}
}

func (c *Client) SearchTracks(ctx context.Context, query string) ([]domainspotify.Track, error) {
	if c.cfg.ClientID == "" || c.cfg.Secret == "" {
		return c.local.SearchLocal(ctx, query)
	}
	token, err := c.tokenValue(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIBase+"/search?type=track&limit=10&q="+url.QueryEscape(query), nil)
	if err != nil {
		return nil, &domainspotify.ProviderError{Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &domainspotify.ProviderError{Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, &domainspotify.ProviderError{StatusCode: resp.StatusCode}
	}
	var payload struct {
		Tracks struct {
			Items []spotifyTrackResponse `json:"items"`
		} `json:"tracks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, &domainspotify.ProviderError{Cause: err}
	}
	out := make([]domainspotify.Track, 0, len(payload.Tracks.Items))
	for _, item := range payload.Tracks.Items {
		out = append(out, toDomainTrack(item))
	}
	return out, nil
}

func (c *Client) GetTrack(ctx context.Context, spotifyTrackID string) (domainspotify.Track, error) {
	if c.cfg.ClientID == "" || c.cfg.Secret == "" {
		_, track, err := c.local.GetBySpotifyID(ctx, spotifyTrackID)
		if err != nil {
			return domainspotify.Track{}, &domainspotify.ProviderError{Cause: err}
		}
		return track, nil
	}
	token, err := c.tokenValue(ctx)
	if err != nil {
		return domainspotify.Track{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.APIBase+"/tracks/"+spotifyTrackID, nil)
	if err != nil {
		return domainspotify.Track{}, &domainspotify.ProviderError{Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domainspotify.Track{}, &domainspotify.ProviderError{Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return domainspotify.Track{}, &domainspotify.ProviderError{StatusCode: resp.StatusCode}
	}
	var item spotifyTrackResponse
	if err := json.NewDecoder(resp.Body).Decode(&item); err != nil {
		return domainspotify.Track{}, &domainspotify.ProviderError{Cause: err}
	}
	return toDomainTrack(item), nil
}

func (c *Client) tokenValue(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().UTC().Before(c.expiresAt.Add(-30*time.Second)) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", &domainspotify.ProviderError{Cause: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.Secret)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", &domainspotify.ProviderError{Cause: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", &domainspotify.ProviderError{StatusCode: resp.StatusCode}
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", &domainspotify.ProviderError{Cause: err}
	}
	c.token = payload.AccessToken
	c.expiresAt = time.Now().UTC().Add(time.Duration(payload.ExpiresIn) * time.Second)
	return c.token, nil
}

type spotifyTrackResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Album struct {
		Name   string `json:"name"`
		Images []struct {
			URL string `json:"url"`
		} `json:"images"`
	} `json:"album"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
	DurationMS int    `json:"duration_ms"`
	PreviewURL string `json:"preview_url"`
	URI        string `json:"uri"`
}

func toDomainTrack(item spotifyTrackResponse) domainspotify.Track {
	raw, _ := json.Marshal(item)
	out := domainspotify.Track{
		SpotifyTrackID: item.ID,
		Title:          item.Name,
		ArtistNames:    strings.Join(artistNames(item.Artists), ", "),
		AlbumName:      item.Album.Name,
		DurationMS:     item.DurationMS,
		PreviewURL:     item.PreviewURL,
		URI:            item.URI,
		RawPayload:     raw,
	}
	if len(item.Album.Images) > 0 {
		out.ImageURL = item.Album.Images[0].URL
	}
	return out
}

func artistNames(artists []struct {
	Name string `json:"name"`
}) []string {
	out := make([]string, 0, len(artists))
	for _, artist := range artists {
		out = append(out, artist.Name)
	}
	return out
}
