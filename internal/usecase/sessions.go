package usecase

import (
	"context"
	"errors"
	"log"
	"net/http"

	"crowdbeats/internal/domain/session"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Services) Heartbeat(ctx context.Context, current session.Session, roomID uuid.UUID) error {
	if roomID != uuid.Nil && roomID != current.RoomID {
		return apierror.New("SESSION_NOT_IN_ROOM", "session is not in requested room", http.StatusForbidden)
	}
	if err := s.Repos.Sessions().Heartbeat(ctx, current.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apierror.New("UNAUTHORIZED", "session inactive", http.StatusUnauthorized)
		}
		return err
	}
	return nil
}

func (s *Services) Leave(ctx context.Context, current session.Session) error {
	if err := s.Repos.Sessions().Leave(ctx, current.ID); err != nil {
		return err
	}
	s.Broadcaster.DisconnectSession(current.RoomID, current.ID)
	if err := s.BroadcastPresence(ctx, current.RoomID); err != nil {
		log.Printf("broadcast presence after leave: %v", err)
	}
	return nil
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
