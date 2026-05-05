package usecase

import (
	"context"
	"net/http"

	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
)

func (s *Services) DeleteTrack(ctx context.Context, roomID, roomTrackID uuid.UUID) error {
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		deleted, err := repos.Tracks().Delete(ctx, roomID, roomTrackID)
		if err != nil {
			return err
		}
		if !deleted {
			return apierror.New("ROOM_TRACK_NOT_FOUND", "track not found", http.StatusNotFound)
		}
		if err := repos.Events().Insert(ctx, roomID, "track_deleted", map[string]any{"room_track_id": roomTrackID}); err != nil {
			return err
		}
		s.DirtyRooms.Mark(roomID)
		s.Broadcaster.Broadcast(LiveEvent{
			Name:      "track_deleted",
			RoomID:    roomID,
			Timestamp: s.Now(),
			Payload:   map[string]any{"room_track_id": roomTrackID},
		})
		return nil
	})
	if err == nil {
		_, _ = s.RecalculateRoomQueue(ctx, roomID)
	}
	return err
}

func (s *Services) SetTrackStatus(ctx context.Context, roomID, roomTrackID uuid.UUID, status, eventName, tsColumn string, recalcNow bool) error {
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		updated, err := repos.Tracks().SetStatus(ctx, roomID, roomTrackID, status, tsColumn)
		if err != nil {
			return err
		}
		if !updated {
			return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track not found or not active", http.StatusNotFound)
		}
		if err := repos.Events().Insert(ctx, roomID, eventName, map[string]any{"room_track_id": roomTrackID}); err != nil {
			return err
		}
		s.DirtyRooms.Mark(roomID)
		s.Broadcaster.Broadcast(LiveEvent{
			Name:      eventName,
			RoomID:    roomID,
			Timestamp: s.Now(),
			Payload:   map[string]any{"room_track_id": roomTrackID},
		})
		return nil
	})
	if err == nil && recalcNow {
		_, _ = s.RecalculateRoomQueue(ctx, roomID)
	}
	return err
}

func (s *Services) SetTrackPlaying(ctx context.Context, roomID, roomTrackID uuid.UUID) error {
	return s.UOW.Run(ctx, func(repos RepositorySet) error {
		updated, err := repos.Tracks().SetPlaying(ctx, roomID, roomTrackID)
		if err != nil {
			return err
		}
		if !updated {
			return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track not found or not active", http.StatusNotFound)
		}
		if err := repos.Events().Insert(ctx, roomID, "track_playing", map[string]any{"room_track_id": roomTrackID}); err != nil {
			return err
		}
		s.DirtyRooms.Mark(roomID)
		s.Broadcaster.Broadcast(LiveEvent{
			Name:      "track_playing",
			RoomID:    roomID,
			Timestamp: s.Now(),
			Payload:   map[string]any{"room_track_id": roomTrackID},
		})
		return nil
	})
}
