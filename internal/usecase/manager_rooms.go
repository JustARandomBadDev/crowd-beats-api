package usecase

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	defaultQueueLimit            = 20
	defaultMaxVotesPerUser       = 5
	defaultQRTTLSeconds          = 14400
	defaultRecalcIntervalSeconds = 10
	minQRTTLSeconds              = 60
	maxQRTTLSeconds              = 604800
)

type CreateRoomInput struct {
	Name                  string
	Slug                  *string
	Status                *string
	QueueLimit            *int
	MaxVotesPerUser       *int
	QRTTLSeconds          *int
	RecalcIntervalSeconds *int
}

func (s *Services) CreateRoom(ctx context.Context, input CreateRoomInput) (room.Room, string, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return room.Room{}, "", validationError("room name is required")
	}
	status := room.StatusDraft
	if input.Status != nil {
		status = *input.Status
	}
	if !validRoomStatus(status) {
		return room.Room{}, "", validationError("invalid room status")
	}
	queueLimit := defaultQueueLimit
	if input.QueueLimit != nil {
		queueLimit = *input.QueueLimit
	}
	if queueLimit < 1 || queueLimit > 200 {
		return room.Room{}, "", validationError("queue_limit must be between 1 and 200")
	}
	maxVotes := defaultMaxVotesPerUser
	if input.MaxVotesPerUser != nil {
		maxVotes = *input.MaxVotesPerUser
	}
	if maxVotes < 1 || maxVotes > 20 {
		return room.Room{}, "", validationError("max_votes_per_user must be between 1 and 20")
	}
	qrTTL := defaultQRTTLSeconds
	if input.QRTTLSeconds != nil {
		qrTTL = *input.QRTTLSeconds
	}
	if qrTTL < minQRTTLSeconds || qrTTL > maxQRTTLSeconds {
		return room.Room{}, "", validationError("qr_ttl_seconds must be between 60 and 604800")
	}
	recalcInterval := defaultRecalcIntervalSeconds
	if input.RecalcIntervalSeconds != nil {
		recalcInterval = *input.RecalcIntervalSeconds
	}
	if recalcInterval < 2 || recalcInterval > 300 {
		return room.Room{}, "", validationError("recalc_interval_seconds must be between 2 and 300")
	}

	managerSecret, err := s.Tokens.NewToken(24)
	if err != nil {
		return room.Room{}, "", err
	}
	created, err := s.Repos.Rooms().Create(ctx, room.CreateInput{
		Name: name, Slug: input.Slug, Status: status,
		QueueLimit: queueLimit, MaxVotesPerUser: maxVotes,
		QRTTLSeconds: qrTTL, RecalcIntervalSeconds: recalcInterval,
		ManagerSecretHash: s.Tokens.Hash(managerSecret),
	})
	return created, managerSecret, err
}

func (s *Services) CreateQRCode(ctx context.Context, roomID uuid.UUID, expiresInSeconds *int) (room.QRCode, error) {
	if expiresInSeconds != nil && (*expiresInSeconds < minQRTTLSeconds || *expiresInSeconds > maxQRTTLSeconds) {
		return room.QRCode{}, validationError("expires_in_seconds must be between 60 and 604800")
	}
	var qr room.QRCode
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		lockedRoom, err := repos.Rooms().GetByIDForUpdate(ctx, roomID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apierror.New("NOT_FOUND", "room not found", http.StatusNotFound)
			}
			return err
		}
		ttl := lockedRoom.QRTTLSeconds
		if expiresInSeconds != nil {
			ttl = *expiresInSeconds
		}
		if ttl < minQRTTLSeconds || ttl > maxQRTTLSeconds {
			return validationError("room QR TTL is invalid")
		}
		code, err := s.Tokens.NewToken(12)
		if err != nil {
			return err
		}
		qr = room.QRCode{
			Code:      "qr_" + code,
			ExpiresAt: s.Now().Add(time.Duration(ttl) * time.Second),
		}
		return repos.Rooms().RotateQRCode(ctx, roomID, qr.Code, qr.ExpiresAt)
	})
	return qr, err
}

func (s *Services) PatchRoom(ctx context.Context, roomID uuid.UUID, patch room.Patch) (room.Room, error) {
	if patch.Status != nil && !validRoomStatus(*patch.Status) {
		return room.Room{}, validationError("invalid room status")
	}
	if patch.QueueLimit != nil && (*patch.QueueLimit < 1 || *patch.QueueLimit > 200) {
		return room.Room{}, validationError("queue_limit must be between 1 and 200")
	}
	if patch.MaxVotesPerUser != nil && (*patch.MaxVotesPerUser < 1 || *patch.MaxVotesPerUser > 20) {
		return room.Room{}, validationError("max_votes_per_user must be between 1 and 20")
	}
	updated, err := s.Repos.Rooms().Patch(ctx, roomID, patch)
	if err == nil {
		s.DirtyRooms.Mark(roomID)
	}
	return updated, err
}

func validRoomStatus(status string) bool {
	switch status {
	case room.StatusDraft, room.StatusActive, room.StatusPaused, room.StatusClosed:
		return true
	default:
		return false
	}
}

func validationError(message string) error {
	return apierror.New("VALIDATION_ERROR", message, http.StatusBadRequest)
}

func (s *Services) GetRoom(ctx context.Context, roomID uuid.UUID) (room.Room, error) {
	return s.Repos.Rooms().GetByID(ctx, roomID)
}

func (s *Services) GetRoomStats(ctx context.Context, roomID uuid.UUID) (room.Stats, error) {
	return s.Repos.Rooms().LoadStats(ctx, roomID)
}
