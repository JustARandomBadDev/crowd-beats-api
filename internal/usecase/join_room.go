package usecase

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type JoinRoomResult struct {
	Room         room.Room
	Session      session.Session
	Token        string
	WebSocketURL string
}

func (s *Services) JoinByQRCode(ctx context.Context, qrCode, nickname, existingToken string) (JoinRoomResult, error) {
	var result JoinRoomResult
	nickname = strings.TrimSpace(nickname)
	if n := utf8.RuneCountInString(nickname); n < 1 || n > 32 {
		return result, validationError("nickname must contain between 1 and 32 characters")
	}
	var oldRoomID uuid.UUID
	var oldSessionID uuid.UUID
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByQRCode(ctx, qrCode)
		if err != nil {
			return publicQRCodeError(err)
		}
		// Rotation and status changes take this row lock. Check the QR again after
		// acquiring it so a concurrent rotation cannot authorize a stale QR.
		lockedRoom, err := repos.Rooms().GetByIDForUpdate(ctx, targetRoom.ID)
		if err != nil {
			return err
		}
		targetRoom, err = repos.Rooms().GetByQRCode(ctx, qrCode)
		if err != nil {
			return publicQRCodeError(err)
		}
		if targetRoom.ID != lockedRoom.ID {
			return publicQRCodeError(pgx.ErrNoRows)
		}
		if lockedRoom.Status != room.StatusActive {
			return apierror.New("ROOM_NOT_ACTIVE", "room is not active", http.StatusForbidden)
		}

		var current session.Session
		if isSessionToken(existingToken) {
			current, err = repos.Sessions().GetByTokenHashForUpdate(ctx, s.Tokens.Hash(existingToken))
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
		}
		if current.ID != uuid.Nil && current.RoomID == lockedRoom.ID {
			if err := repos.Sessions().ReattachSameRoom(ctx, current.ID, lockedRoom.ID, nickname); err != nil {
				return err
			}
			current.Nickname = nickname
			current.Role = session.RoleGuest
			current.Status = session.StatusActive
			current.LastSeenAt = s.Now()
			result.Session = current
			result.Token = existingToken
		} else {
			if current.ID != uuid.Nil {
				if err := repos.Sessions().Leave(ctx, current.ID); err != nil {
					return err
				}
				oldRoomID = current.RoomID
				oldSessionID = current.ID
			}
			newToken, err := s.Tokens.NewToken(32)
			if err != nil {
				return err
			}
			created, err := repos.Sessions().Create(ctx, session.CreateInput{
				RoomID: lockedRoom.ID, SessionTokenHash: s.Tokens.Hash(newToken),
				Nickname: nickname, Role: session.RoleGuest,
			})
			if err != nil {
				return err
			}
			result.Session = created
			result.Token = newToken
		}

		if err := repos.Events().Insert(ctx, lockedRoom.ID, "user_joined", map[string]any{
			"session_id": result.Session.ID,
			"nickname":   nickname,
		}); err != nil {
			return err
		}

		result.Room = lockedRoom
		result.WebSocketURL = "/ws?room_id=" + lockedRoom.ID.String()
		return nil
	})
	if err != nil {
		return JoinRoomResult{}, err
	}
	if oldSessionID != uuid.Nil {
		s.Broadcaster.DisconnectSession(oldRoomID, oldSessionID)
	}

	roomID := result.Room.ID
	s.Broadcaster.Broadcast(LiveEvent{
		Name:      "room_joined",
		RoomID:    roomID,
		Timestamp: s.Now(),
		Payload: map[string]any{
			"session_id": result.Session.ID,
			"nickname":   result.Session.Nickname,
		},
	})
	_ = s.BroadcastPresence(ctx, roomID)
	if oldRoomID != uuid.Nil {
		_ = s.BroadcastPresence(ctx, oldRoomID)
	}
	return result, nil
}

func isSessionToken(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func publicQRCodeError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apierror.New("INVALID_QR_CODE", "qr code is invalid, expired, or revoked", http.StatusForbidden)
	}
	return err
}
