package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/infra/ws"
	"crowdbeats/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRouterHealthLive(t *testing.T) {
	h := testutil.NewHarness()
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry())

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"ok"`)
}

func TestRouterSessionMeRequiresBearerToken(t *testing.T) {
	h := testutil.NewHarness()
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"UNAUTHORIZED"`)
}

func TestRouterCreateRoomReturnsManagerSecret(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	h.Tokens.NewTokenValue = "manager-secret"
	h.Repos.RoomRepo.CreateFn = func(_ context.Context, input room.CreateInput) (room.Room, error) {
		return room.Room{
			ID:              roomID,
			Name:            input.Name,
			Status:          input.Status,
			QueueLimit:      input.QueueLimit,
			MaxVotesPerUser: input.MaxVotesPerUser,
		}, nil
	}
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry())

	body, err := json.Marshal(map[string]any{
		"name": "Le Neon",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/manager/rooms", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), `"manager_secret":"manager-secret"`)
	require.Contains(t, rec.Body.String(), `"Name":"Le Neon"`)
}
