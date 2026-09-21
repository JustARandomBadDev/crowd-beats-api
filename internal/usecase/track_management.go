package usecase

import (
	"context"
	"net/http"

	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
)

func (s *Services) DeleteTrack(ctx context.Context, roomID, roomTrackID uuid.UUID) error {
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		if _, err := repos.Rooms().GetByIDForUpdate(ctx, roomID); err != nil {
			return err
		}
		deleted, err := repos.Tracks().Delete(ctx, roomID, roomTrackID)
		if err != nil {
			return err
		}
		if !deleted {
			return apierror.New("ROOM_TRACK_NOT_FOUND", "track not found", http.StatusNotFound)
		}
		return repos.Events().Insert(ctx, roomID, "track_deleted", map[string]any{"room_track_id": roomTrackID})
	})
	if err != nil {
		return err
	}
	s.Broadcaster.Broadcast(LiveEvent{
		Name: "track_deleted", RoomID: roomID, Timestamp: s.Now(),
		Payload: map[string]any{"room_track_id": roomTrackID},
	})
	return nil
}

func (s *Services) SetTrackStatus(ctx context.Context, roomID, roomTrackID uuid.UUID, status, eventName, tsColumn string) error {
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		if _, err := repos.Rooms().GetByIDForUpdate(ctx, roomID); err != nil {
			return err
		}
		updated, err := repos.Tracks().SetStatus(ctx, roomID, roomTrackID, status, tsColumn)
		if err != nil {
			return err
		}
		if !updated {
			return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track not found or not active", http.StatusNotFound)
		}
		return repos.Events().Insert(ctx, roomID, eventName, map[string]any{"room_track_id": roomTrackID})
	})
	if err != nil {
		return err
	}
	s.Broadcaster.Broadcast(LiveEvent{
		Name: eventName, RoomID: roomID, Timestamp: s.Now(),
		Payload: map[string]any{"room_track_id": roomTrackID},
	})
	return nil
}

func (s *Services) SetTrackPlaying(ctx context.Context, roomID, roomTrackID uuid.UUID) error {
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		if _, err := repos.Rooms().GetByIDForUpdate(ctx, roomID); err != nil {
			return err
		}
		updated, err := repos.Tracks().SetPlaying(ctx, roomID, roomTrackID)
		if err != nil {
			return err
		}
		if !updated {
			return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track not found or not active", http.StatusNotFound)
		}
		return repos.Events().Insert(ctx, roomID, "track_playing", map[string]any{"room_track_id": roomTrackID})
	})
	if err != nil {
		return err
	}
	s.Broadcaster.Broadcast(LiveEvent{
		Name: "track_playing", RoomID: roomID, Timestamp: s.Now(),
		Payload: map[string]any{"room_track_id": roomTrackID},
	})
	return nil
}
