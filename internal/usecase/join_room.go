package usecase

import (
	"context"
	"errors"
	"net/http"

	"crowdbeats/internal/domain/session"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type JoinRoomResult struct {
	Room         map[string]any
	Session      map[string]any
	WebSocketURL string
}

func (s *Services) JoinByQRCode(ctx context.Context, qrCode, nickname, existingToken string) (JoinRoomResult, error) {
	var result JoinRoomResult
	token := existingToken
	if token == "" {
		var err error
		token, err = s.Tokens.NewToken(32)
		if err != nil {
			return result, err
		}
	}

	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByQRCode(ctx, qrCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apierror.New("INVALID_QR_CODE", "qr code is invalid, expired, or revoked", http.StatusForbidden)
			}
			return err
		}

		hashed := s.Tokens.Hash(token)
		current, err := repos.Sessions().GetByTokenHash(ctx, hashed)
		switch {
		case err == nil:
			if err := repos.Sessions().Reattach(ctx, current.ID, targetRoom.ID, nickname); err != nil {
				return err
			}
			current.RoomID = targetRoom.ID
			current.Nickname = nickname
			current.Role = session.RoleGuest
			current.Status = session.StatusActive
			result.Session = map[string]any{
				"id":       current.ID,
				"nickname": current.Nickname,
				"role":     current.Role,
				"token":    token,
			}
		case errors.Is(err, pgx.ErrNoRows):
			current, err = repos.Sessions().Create(ctx, session.CreateInput{
				RoomID:           targetRoom.ID,
				SessionTokenHash: hashed,
				Nickname:         nickname,
				Role:             session.RoleGuest,
			})
			if err != nil {
				return err
			}
			result.Session = map[string]any{
				"id":       current.ID,
				"nickname": current.Nickname,
				"role":     current.Role,
				"token":    token,
			}
		default:
			return err
		}

		if err := repos.Events().Insert(ctx, targetRoom.ID, "user_joined", map[string]any{
			"session_id": result.Session["id"],
			"nickname":   nickname,
		}); err != nil {
			return err
		}

		result.Room = map[string]any{
			"id":                 targetRoom.ID,
			"name":               targetRoom.Name,
			"status":             targetRoom.Status,
			"queue_limit":        targetRoom.QueueLimit,
			"max_votes_per_user": targetRoom.MaxVotesPerUser,
		}
		result.WebSocketURL = "/ws?room_id=" + targetRoom.ID.String()
		return nil
	})
	if err != nil {
		return result, err
	}

	roomID := result.Room["id"].(uuid.UUID)
	s.Broadcaster.Broadcast(LiveEvent{
		Name:      "room_joined",
		RoomID:    roomID,
		Timestamp: s.Now(),
		Payload: map[string]any{
			"session_id": result.Session["id"],
			"nickname":   result.Session["nickname"],
		},
	})
	_ = s.BroadcastPresence(ctx, roomID)
	return result, nil
}
