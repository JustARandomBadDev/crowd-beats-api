package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/infra/ws"
	"crowdbeats/internal/testutil"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type testPinger func(context.Context) error

func (p testPinger) Ping(ctx context.Context) error { return p(ctx) }

func TestRouterHealthLive(t *testing.T) {
	h := testutil.NewHarness()
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), nil)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"ok"`)
}

func TestRouterReadinessChecksPostgreSQLOnly(t *testing.T) {
	h := testutil.NewHarness()
	request := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	healthy := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), testPinger(func(ctx context.Context) error {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 2*time.Second)
		return nil
	}))
	response := httptest.NewRecorder()
	healthy.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"data":{"status":"ready"},"error":null,"meta":{}}`, response.Body.String())

	unavailable := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), testPinger(func(context.Context) error {
		return errors.New("private-db-url-and-password")
	}))
	response = httptest.NewRecorder()
	unavailable.ServeHTTP(response, request)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.JSONEq(t, `{"data":{"status":"not_ready"},"error":null,"meta":{}}`, response.Body.String())
	require.NotContains(t, response.Body.String(), "private-db")

	response = httptest.NewRecorder()
	unavailable.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"data":{"status":"ok"},"error":null,"meta":{}}`, response.Body.String())
}

func TestRouterSessionMeRequiresBearerToken(t *testing.T) {
	h := testutil.NewHarness()
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), nil)

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
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), nil)

	body, err := json.Marshal(map[string]any{
		"name": "Le Neon",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/manager/rooms", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.Contains(t, rec.Body.String(), `"manager_secret":"manager-secret"`)
	require.Contains(t, rec.Body.String(), `"name":"Le Neon"`)
}
