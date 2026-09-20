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

type ProposedRoomTrack struct {
	ID       uuid.UUID
	Status   string
	Position *int
}

type ProposeTrackResult struct {
	RoomTrack         *ProposedRoomTrack
	ExistingRoomTrack *track.DuplicateInfo
	Duplicate         bool
}

func (s *Services) ProposeTrack(ctx context.Context, roomID uuid.UUID, current session.Session, spotifyTrackID string) (ProposeTrackResult, int, error) {
	var response ProposeTrackResult
	status := http.StatusCreated
	if !validSpotifyTrackID(spotifyTrackID) {
		return response, status, apierror.New("INVALID_SPOTIFY_TRACK_ID", "invalid Spotify track ID", http.StatusBadRequest)
	}
	if current.RoomID != roomID || current.Status != session.StatusActive {
		return response, status, apierror.New("SESSION_NOT_IN_ROOM", "session is not active in room", http.StatusForbidden)
	}

	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByID(ctx, roomID)
		if err != nil {
			return err
		}
		if targetRoom.Status != room.StatusActive {
			return apierror.New("ROOM_NOT_ACTIVE", "room is not active", http.StatusForbidden)
		}

		catalogID, catalogTrack, err := repos.Spotify().GetBySpotifyID(ctx, spotifyTrackID)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			catalogTrack, err = s.SpotifyProvider.GetTrack(ctx, spotifyTrackID)
			if err != nil {
				return publicSpotifyTrackError(err)
			}
			catalogID, err = repos.Spotify().Upsert(ctx, catalogTrack)
			if err != nil {
				return err
			}
		}
		catalogUUID, err := uuid.Parse(catalogID)
		if err != nil {
			return err
		}

		// Serialize proposals with room status changes and other proposals. The
		// catalogue lookup above may call Spotify, so acquire the lock afterwards.
		targetRoom, err = repos.Rooms().GetByIDForUpdate(ctx, roomID)
		if err != nil {
			return err
		}
		if targetRoom.Status != room.StatusActive {
			return apierror.New("ROOM_NOT_ACTIVE", "room is not active", http.StatusForbidden)
		}
		lockedSession, err := repos.Sessions().GetByIDForUpdate(ctx, current.ID)
		if err != nil {
			return err
		}
		if lockedSession.RoomID != roomID || lockedSession.Status != session.StatusActive {
			return apierror.New("SESSION_NOT_IN_ROOM", "session is not active in room", http.StatusForbidden)
		}

		duplicate, err := repos.Tracks().GetActiveDuplicate(ctx, roomID, catalogUUID)
		if err == nil {
			response = ProposeTrackResult{Duplicate: true, ExistingRoomTrack: &duplicate}
			status = http.StatusOK
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		activeCount, err := repos.Tracks().CountActive(ctx, roomID)
		if err != nil {
			return err
		}
		if activeCount >= targetRoom.QueueLimit {
			return apierror.New("QUEUE_FULL", "queue is full", http.StatusConflict)
		}

		proposed, created, err := repos.Tracks().CreateQueuedIfAbsent(ctx, roomID, catalogUUID, current.ID)
		if err != nil {
			return err
		}
		if !created {
			duplicate, err := repos.Tracks().GetActiveDuplicate(ctx, roomID, catalogUUID)
			if err != nil {
				return err
			}
			response = ProposeTrackResult{Duplicate: true, ExistingRoomTrack: &duplicate}
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

		response = ProposeTrackResult{RoomTrack: &ProposedRoomTrack{ID: proposed.ID, Status: track.StatusQueued}}

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

func validSpotifyTrackID(id string) bool {
	if len(id) != 22 {
		return false
	}
	for _, char := range id {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9') {
			return false
		}
	}
	return true
}
