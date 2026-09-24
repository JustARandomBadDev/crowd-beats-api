package usecase_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/testutil"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestHeartbeatNeverChangesRoom(t *testing.T) {
	h := testutil.NewHarness()
	roomID, sessionID := uuid.New(), uuid.New()
	current := session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}
	called := 0
	h.Repos.SessionRepo.HeartbeatFn = func(_ context.Context, id uuid.UUID) error {
		require.Equal(t, sessionID, id)
		called++
		return nil
	}
	require.NoError(t, h.Services.Heartbeat(context.Background(), current, roomID))
	require.NoError(t, h.Services.Heartbeat(context.Background(), current, uuid.Nil))
	require.Equal(t, 2, called)
	err := h.Services.Heartbeat(context.Background(), current, uuid.New())
	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "SESSION_NOT_IN_ROOM", apiErr.Code)
	require.Equal(t, http.StatusForbidden, apiErr.Status)
	require.Equal(t, 2, called)
}

func TestHeartbeatCannotReactivateLeftSessionAfterConcurrentSwitch(t *testing.T) {
	h := testutil.NewHarness()
	h.Repos.SessionRepo.HeartbeatFn = func(context.Context, uuid.UUID) error { return pgx.ErrNoRows }
	err := h.Services.Heartbeat(context.Background(), session.Session{ID: uuid.New(), Status: session.StatusActive}, uuid.Nil)
	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "UNAUTHORIZED", apiErr.Code)
	require.Equal(t, http.StatusUnauthorized, apiErr.Status)
}

func TestLeaveClosesSessionSocketsOnlyAfterPersistence(t *testing.T) {
	h := testutil.NewHarness()
	roomID, sessionID := uuid.New(), uuid.New()
	left := false
	h.Repos.SessionRepo.LeaveFn = func(context.Context, uuid.UUID) error {
		left = true
		return nil
	}
	h.Broadcaster.DisconnectFn = func(actualRoom, actualSession uuid.UUID) {
		require.True(t, left)
		require.Equal(t, roomID, actualRoom)
		require.Equal(t, sessionID, actualSession)
	}
	require.NoError(t, h.Services.Leave(context.Background(), session.Session{ID: sessionID, RoomID: roomID}))
	require.Len(t, h.Broadcaster.Disconnected, 1)
	require.Equal(t, "presence_updated", h.Broadcaster.Events[0].Name)

	h = testutil.NewHarness()
	h.Repos.SessionRepo.LeaveFn = func(context.Context, uuid.UUID) error { return errors.New("DB failed") }
	require.Error(t, h.Services.Leave(context.Background(), session.Session{ID: sessionID, RoomID: roomID}))
	require.Empty(t, h.Broadcaster.Disconnected)
	require.Empty(t, h.Broadcaster.Events)
}
