package usecase

import (
	"context"
	"errors"
	"net/http"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/track"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Services) ProposeTrack(ctx context.Context, roomID uuid.UUID, current session.Session, spotifyTrackID string) (map[string]any, int, error) {
	var response map[string]any
	status := http.StatusCreated

	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByID(ctx, roomID)
		if err != nil {
			return err
		}
		if targetRoom.Status != room.StatusActive {
			return apierror.New("ROOM_INACTIVE", "room is not active", http.StatusForbidden)
		}

		activeCount, err := repos.Tracks().CountActive(ctx, roomID)
		if err != nil {
			return err
		}
		if activeCount >= targetRoom.QueueLimit {
			return apierror.New("QUEUE_FULL", "queue is full", http.StatusConflict)
		}

		catalogID, catalogTrack, err := repos.Spotify().GetBySpotifyID(ctx, spotifyTrackID)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			catalogTrack, err = s.SpotifyProvider.GetTrack(ctx, spotifyTrackID)
			if err != nil {
				return apierror.New("SPOTIFY_TRACK_NOT_FOUND", err.Error(), http.StatusBadGateway)
			}
			catalogID, err = repos.Spotify().Upsert(ctx, catalogTrack)
			if err != nil {
				return err
			}
		}

		proposed, err := repos.Tracks().CreateQueued(ctx, roomID, uuid.MustParse(catalogID), current.ID)
		if err != nil {
			if !isUniqueConstraint(err, "uq_room_tracks_active_unique_track") {
				return err
			}
			duplicate, err := repos.Tracks().GetActiveDuplicate(ctx, roomID, uuid.MustParse(catalogID))
			if err != nil {
				return err
			}
			response = map[string]any{
				"duplicate": true,
				"existing_room_track": map[string]any{
					"id":                 duplicate.ID,
					"current_vote_count": duplicate.CurrentVoteCount,
					"position":           duplicate.Position,
				},
			}
			status = http.StatusOK
			return nil
		}

		if err := repos.Events().Insert(ctx, roomID, "track_proposed", map[string]any{
			"room_track_id":    proposed.ID,
			"spotify_track_id": catalogTrack.SpotifyTrackID,
			"title":            catalogTrack.Title,
			"artist_names":     catalogTrack.ArtistNames,
		}); err != nil {
			return err
		}

		response = map[string]any{
			"room_track": map[string]any{
				"id":       proposed.ID,
				"status":   track.StatusQueued,
				"position": nil,
			},
			"duplicate": false,
		}

		s.DirtyRooms.Mark(roomID)
		s.Broadcaster.Broadcast(LiveEvent{
			Name:      "track_proposed",
			RoomID:    roomID,
			Timestamp: s.Now(),
			Payload: map[string]any{
				"room_track_id":    proposed.ID,
				"spotify_track_id": catalogTrack.SpotifyTrackID,
				"title":            catalogTrack.Title,
				"artist_names":     catalogTrack.ArtistNames,
			},
		})
		return nil
	})
	return response, status, err
}
