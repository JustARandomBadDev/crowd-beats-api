//go:build integration

package repositories_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"crowdbeats/internal/app"
	"crowdbeats/internal/infra/db"
	"crowdbeats/internal/infra/ws"
	"crowdbeats/internal/platform/auth"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHTTPCreateRoomAndReadinessWithPostgreSQL(t *testing.T) {
	pool := testPool(t)
	store := db.NewStore(pool)
	registry := ws.NewRegistry()
	services := usecase.NewServices(store, store, nil, registry, nil, auth.NewTokenManager())
	router := app.NewRouter(app.Config{AllowedOrigins: "*"}, services, registry, pool)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/manager/rooms", bytes.NewBufferString(`{"name":"Integration Room"}`))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusCreated, response.Code)
	var created struct {
		Data struct {
			Room struct {
				ID                    uuid.UUID `json:"id"`
				RecalcIntervalSeconds int       `json:"recalc_interval_seconds"`
			} `json:"room"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &created))
	require.NotEqual(t, uuid.Nil, created.Data.Room.ID)
	require.Equal(t, 10, created.Data.Room.RecalcIntervalSeconds)
	require.NotContains(t, response.Body.String(), "manager_secret_hash")

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/rooms/"+created.Data.Room.ID.String(), nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"name":"Integration Room"`)

	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"data":{"status":"ready"},"error":null,"meta":{}}`, response.Body.String())
}
