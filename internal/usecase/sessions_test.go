package usecase_test

import (
	"context"
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
