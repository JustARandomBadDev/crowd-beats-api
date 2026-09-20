package usecase_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/testutil"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

const knownSessionToken = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const freshSessionToken = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func joinHarness(targetRoom room.Room) *testutil.Harness {
	h := testutil.NewHarness()
	h.Tokens.NewTokenValue = freshSessionToken
	h.Repos.RoomRepo.GetByQRCodeFn = func(context.Context, string) (room.Room, error) {
		return targetRoom, nil
	}
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return targetRoom, nil
	}
	return h
}

func TestJoinByQRCodeRejectsInvalidQRCode(t *testing.T) {
	h := testutil.NewHarness()
	h.Repos.RoomRepo.GetByQRCodeFn = func(context.Context, string) (room.Room, error) {
		return room.Room{}, pgx.ErrNoRows
	}

	_, err := h.Services.JoinByQRCode(context.Background(), "invalid", "alex", "")
	require.Error(t, err)

	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "INVALID_QR_CODE", apiErr.Code)
	require.Equal(t, http.StatusForbidden, apiErr.Status)
}

func TestJoinByQRCodeRejectsBlankNickname(t *testing.T) {
	h := joinHarness(room.Room{ID: uuid.New(), Status: room.StatusActive})
	for _, nickname := range []string{"", "   "} {
		_, err := h.Services.JoinByQRCode(context.Background(), "qr_valid", nickname, "")
		var apiErr apierror.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, "VALIDATION_ERROR", apiErr.Code)
		require.Equal(t, http.StatusBadRequest, apiErr.Status)
	}
}

func TestJoinByQRCodeAlwaysGeneratesNewTokenForNewOrUnknownSession(t *testing.T) {
	for _, offeredToken := range []string{"", knownSessionToken, "abc", strings.Repeat("g", 64)} {
		t.Run(offeredToken, func(t *testing.T) {
			roomID := uuid.New()
			h := joinHarness(room.Room{ID: roomID, Status: room.StatusActive})
			lookups := 0
			h.Repos.SessionRepo.GetByTokenHashForUpdateFn = func(context.Context, string) (session.Session, error) {
				lookups++
				return session.Session{}, pgx.ErrNoRows
			}
			sessionID := uuid.New()
			h.Repos.SessionRepo.CreateFn = func(_ context.Context, input session.CreateInput) (session.Session, error) {
				require.Equal(t, "hashed:"+freshSessionToken, input.SessionTokenHash)
				require.Equal(t, roomID, input.RoomID)
				return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
			}
			result, err := h.Services.JoinByQRCode(context.Background(), "qr_valid", "Alex", offeredToken)
			require.NoError(t, err)
			require.Equal(t, freshSessionToken, result.Token)
			require.Equal(t, sessionID, result.Session.ID)
			if offeredToken == knownSessionToken {
				require.Equal(t, 1, lookups)
			} else {
				require.Zero(t, lookups)
			}
		})
	}
}

func TestJoinByQRCodeReattachesSessionInSameRoom(t *testing.T) {
	for _, oldStatus := range []string{session.StatusActive, session.StatusLeft} {
		roomID, sessionID := uuid.New(), uuid.New()
		h := joinHarness(room.Room{ID: roomID, Status: room.StatusActive})
		original := session.Session{ID: sessionID, RoomID: roomID, Status: oldStatus, Nickname: "Old"}
		h.Repos.SessionRepo.GetByTokenHashForUpdateFn = func(_ context.Context, hash string) (session.Session, error) {
			require.Equal(t, "hashed:"+knownSessionToken, hash)
			return original, nil
		}
		reattached := false
		h.Repos.SessionRepo.ReattachSameRoomFn = func(_ context.Context, id, requestedRoomID uuid.UUID, nickname string) error {
			require.Equal(t, sessionID, id)
			require.Equal(t, roomID, requestedRoomID)
			require.Equal(t, "New", nickname)
			reattached = true
			return nil
		}
		h.Repos.SessionRepo.CreateFn = func(context.Context, session.CreateInput) (session.Session, error) {
			t.Fatal("rejoin must reuse session")
			return session.Session{}, nil
		}
		result, err := h.Services.JoinByQRCode(context.Background(), "qr_valid", "New", knownSessionToken)
		require.NoError(t, err)
		require.True(t, reattached)
		require.Equal(t, sessionID, result.Session.ID)
		require.Equal(t, knownSessionToken, result.Token)
		require.Equal(t, session.StatusActive, result.Session.Status)
		require.Equal(t, h.Services.Now(), result.Session.LastSeenAt)
		require.Equal(t, roomID, original.RoomID)
	}
}

func TestJoinByQRCodeSwitchesRoomWithoutChangingOldSession(t *testing.T) {
	oldRoomID, newRoomID := uuid.New(), uuid.New()
	oldID, newID := uuid.New(), uuid.New()
	h := joinHarness(room.Room{ID: newRoomID, Status: room.StatusActive})
	old := session.Session{ID: oldID, RoomID: oldRoomID, Status: session.StatusActive, Nickname: "Old"}
	h.Repos.SessionRepo.GetByTokenHashForUpdateFn = func(context.Context, string) (session.Session, error) {
		return old, nil
	}
	left := false
	h.Repos.SessionRepo.LeaveFn = func(_ context.Context, id uuid.UUID) error {
		require.Equal(t, oldID, id)
		left = true
		return nil
	}
	h.Repos.SessionRepo.CreateFn = func(_ context.Context, input session.CreateInput) (session.Session, error) {
		require.True(t, left, "old session must leave before new one is created")
		require.Equal(t, newRoomID, input.RoomID)
		require.Equal(t, "hashed:"+freshSessionToken, input.SessionTokenHash)
		return session.Session{ID: newID, RoomID: newRoomID, Status: session.StatusActive}, nil
	}
	result, err := h.Services.JoinByQRCode(context.Background(), "qr_valid", "New", knownSessionToken)
	require.NoError(t, err)
	require.Equal(t, newID, result.Session.ID)
	require.Equal(t, freshSessionToken, result.Token)
	require.Equal(t, oldRoomID, old.RoomID)
	require.Equal(t, "Old", old.Nickname)
	require.True(t, left)
	require.Len(t, h.Broadcaster.Events, 3) // join and presence for both rooms
}

func TestJoinByQRCodeRequiresActiveRoomEvenWithExistingSession(t *testing.T) {
	for _, status := range []string{room.StatusDraft, room.StatusPaused, room.StatusClosed} {
		t.Run(status, func(t *testing.T) {
			h := joinHarness(room.Room{ID: uuid.New(), Status: status})
			h.Repos.SessionRepo.GetByTokenHashForUpdateFn = func(context.Context, string) (session.Session, error) {
				t.Fatal("inactive room must be rejected before session lookup")
				return session.Session{}, nil
			}
			_, err := h.Services.JoinByQRCode(context.Background(), "qr_valid", "Alex", knownSessionToken)
			var apiErr apierror.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, "ROOM_NOT_ACTIVE", apiErr.Code)
			require.Equal(t, http.StatusForbidden, apiErr.Status)
		})
	}
}

func TestJoinByQRCodeRevalidatesRotatedQR(t *testing.T) {
	roomID := uuid.New()
	h := joinHarness(room.Room{ID: roomID, Status: room.StatusActive})
	reads := 0
	h.Repos.RoomRepo.GetByQRCodeFn = func(context.Context, string) (room.Room, error) {
		reads++
		if reads == 2 {
			return room.Room{}, pgx.ErrNoRows
		}
		return room.Room{ID: roomID, Status: room.StatusActive}, nil
	}
	_, err := h.Services.JoinByQRCode(context.Background(), "qr_rotated", "Alex", knownSessionToken)
	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "INVALID_QR_CODE", apiErr.Code)
}
