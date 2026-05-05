package usecase

import (
	"context"
	"net/http"

	"crowdbeats/internal/domain/session"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
)

func (s *Services) Heartbeat(ctx context.Context, current session.Session, roomID uuid.UUID) error {
	if roomID != uuid.Nil && roomID != current.RoomID {
		return apierror.New("SESSION_NOT_IN_ROOM", "session is not in requested room", http.StatusForbidden)
	}
	return s.Repos.Sessions().Heartbeat(ctx, current.ID)
}

func (s *Services) Leave(ctx context.Context, current session.Session) error {
	if err := s.Repos.Sessions().Leave(ctx, current.ID); err != nil {
		return err
	}
	return s.BroadcastPresence(ctx, current.RoomID)
}

func (s *Services) BroadcastPresence(ctx context.Context, roomID uuid.UUID) error {
	activeUsers, err := s.Repos.Rooms().CountActiveUsers(ctx, roomID)
	if err != nil {
		return err
	}
	s.Broadcaster.Broadcast(LiveEvent{
		Name:      "presence_updated",
		RoomID:    roomID,
		Timestamp: s.Now(),
		Payload: map[string]any{
			"active_users": activeUsers,
		},
	})
	return nil
}
