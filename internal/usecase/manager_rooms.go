package usecase

import (
	"context"
	"time"

	"crowdbeats/internal/domain/room"

	"github.com/google/uuid"
)

func (s *Services) CreateRoom(ctx context.Context, input room.CreateInput) (room.Room, string, error) {
	managerSecret, err := s.Tokens.NewToken(24)
	if err != nil {
		return room.Room{}, "", err
	}
	input.ManagerSecretHash = s.Tokens.Hash(managerSecret)
	created, err := s.Repos.Rooms().Create(ctx, input)
	return created, managerSecret, err
}

func (s *Services) CreateQRCode(ctx context.Context, roomID uuid.UUID, expiresInSeconds int, fallbackTTL int) (room.QRCode, error) {
	code, err := s.Tokens.NewToken(12)
	if err != nil {
		return room.QRCode{}, err
	}
	if expiresInSeconds == 0 {
		expiresInSeconds = fallbackTTL
	}
	qr := room.QRCode{
		Code:      "qr_" + code,
		ExpiresAt: s.Now().Add(time.Duration(expiresInSeconds) * time.Second),
	}
	err = s.Repos.Rooms().CreateQRCode(ctx, roomID, qr.Code, qr.ExpiresAt)
	return qr, err
}

func (s *Services) PatchRoom(ctx context.Context, roomID uuid.UUID, patch room.Patch) (room.Room, error) {
	updated, err := s.Repos.Rooms().Patch(ctx, roomID, patch)
	if err == nil {
		s.DirtyRooms.Mark(roomID)
	}
	return updated, err
}

func (s *Services) GetRoom(ctx context.Context, roomID uuid.UUID) (room.Room, error) {
	return s.Repos.Rooms().GetByID(ctx, roomID)
}

func (s *Services) GetRoomStats(ctx context.Context, roomID uuid.UUID) (room.Stats, error) {
	return s.Repos.Rooms().LoadStats(ctx, roomID)
}
