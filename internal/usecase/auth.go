package usecase

import (
	"context"
	"errors"
	"net/http"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Services) AuthenticateSession(ctx context.Context, token string) (session.Session, error) {
	if token == "" {
		return session.Session{}, apierror.New("UNAUTHORIZED", "missing session token", http.StatusUnauthorized)
	}
	current, err := s.Repos.Sessions().GetByTokenHash(ctx, s.Tokens.Hash(token))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return session.Session{}, apierror.New("UNAUTHORIZED", "session not found", http.StatusUnauthorized)
		}
		return session.Session{}, err
	}
	if current.Status != session.StatusActive {
		return session.Session{}, apierror.New("UNAUTHORIZED", "session inactive", http.StatusUnauthorized)
	}
	return current, nil
}

func (s *Services) AuthenticateManager(ctx context.Context, roomID uuid.UUID, secret string) (room.Room, error) {
	current, err := s.Repos.Rooms().GetByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return room.Room{}, apierror.New("MANAGER_FORBIDDEN", "room not found", http.StatusForbidden)
		}
		return room.Room{}, err
	}
	if current.ManagerSecretHash == nil || secret == "" || s.Tokens.Hash(secret) != *current.ManagerSecretHash {
		return room.Room{}, apierror.New("MANAGER_FORBIDDEN", "invalid manager secret", http.StatusForbidden)
	}
	return current, nil
}
