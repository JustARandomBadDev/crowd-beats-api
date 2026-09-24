package repositories

import (
	"context"

	"crowdbeats/internal/domain/spotify"

	"github.com/google/uuid"
)

type SpotifyRepository struct{ q Querier }

func (r *SpotifyRepository) GetBySpotifyID(ctx context.Context, spotifyTrackID string) (string, spotify.Track, error) {
	var id uuid.UUID
	var out spotify.Track
	err := r.q.QueryRow(ctx, `
		select id, spotify_track_id, title, artist_names, coalesce(album_name,''), coalesce(duration_ms,0), coalesce(image_url,''), coalesce(preview_url,''), coalesce(uri,''), raw_payload
		from spotify_tracks
		where spotify_track_id = $1
	`, spotifyTrackID).Scan(&id, &out.SpotifyTrackID, &out.Title, &out.ArtistNames, &out.AlbumName, &out.DurationMS, &out.ImageURL, &out.PreviewURL, &out.URI, &out.RawPayload)
	return id.String(), out, err
}

func (r *SpotifyRepository) Upsert(ctx context.Context, track spotify.Track) (string, error) {
	var id uuid.UUID
	err := r.q.QueryRow(ctx, `
		insert into spotify_tracks (spotify_track_id, title, artist_names, album_name, duration_ms, image_url, preview_url, uri, raw_payload)
		values ($1, $2, $3, nullif($4,''), nullif($5,0), nullif($6,''), nullif($7,''), nullif($8,''), $9)
		on conflict (spotify_track_id) do update
		set title = excluded.title,
		    artist_names = excluded.artist_names,
		    album_name = excluded.album_name,
		    duration_ms = excluded.duration_ms,
		    image_url = excluded.image_url,
		    preview_url = excluded.preview_url,
		    uri = excluded.uri,
		    raw_payload = excluded.raw_payload
		returning id
	`, track.SpotifyTrackID, track.Title, track.ArtistNames, track.AlbumName, track.DurationMS, track.ImageURL, track.PreviewURL, track.URI, []byte(track.RawPayload)).Scan(&id)
	return id.String(), err
}

func (r *SpotifyRepository) SearchLocal(ctx context.Context, query string) ([]spotify.Track, error) {
	rows, err := r.q.Query(ctx, `
		select spotify_track_id, title, artist_names, coalesce(album_name,''), coalesce(duration_ms,0), coalesce(image_url,''), coalesce(preview_url,''), coalesce(uri,''), raw_payload
		from spotify_tracks
		where title ilike $1 or artist_names ilike $1 or spotify_track_id ilike $1
		order by updated_at desc
		limit 10
	`, "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]spotify.Track, 0)
	for rows.Next() {
		var track spotify.Track
		if err := rows.Scan(&track.SpotifyTrackID, &track.Title, &track.ArtistNames, &track.AlbumName, &track.DurationMS, &track.ImageURL, &track.PreviewURL, &track.URI, &track.RawPayload); err != nil {
			return nil, err
		}
		out = append(out, track)
	}
	return out, rows.Err()
}
