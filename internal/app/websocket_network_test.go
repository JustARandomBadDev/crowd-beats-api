package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/infra/ws"
	"crowdbeats/internal/testutil"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestWebSocketSyncAndLeaveClosesConnectionOverNetwork(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local network sockets unavailable: %v", err)
	}
	h := testutil.NewHarness()
	roomID, sessionID := uuid.New(), uuid.New()
	h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
		return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
	}
	registry := ws.NewRegistry()
	h.Services.Broadcaster = registry
	server := &http.Server{Handler: NewRouter(Config{AllowedOrigins: "*"}, h.Services, registry, nil)}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	})

	headers := http.Header{"Authorization": []string{"Bearer valid-token"}}
	connection, response, err := websocket.DefaultDialer.Dial("ws://"+listener.Addr().String()+"/ws?room_id="+roomID.String(), headers)
	require.NoError(t, err)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	defer connection.Close()
	require.NoError(t, connection.SetReadDeadline(time.Now().Add(2*time.Second)))
	_, first, err := connection.ReadMessage()
	require.NoError(t, err)
	var event struct {
		Event string `json:"event"`
	}
	require.NoError(t, json.Unmarshal(first, &event))
	require.Equal(t, "sync_required", event.Event)

	request, err := http.NewRequest(http.MethodPost, "http://"+listener.Addr().String()+"/api/v1/sessions/leave", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer valid-token")
	response, err = http.DefaultClient.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	_, _, err = connection.ReadMessage()
	require.Error(t, err, "leave must close the old WebSocket")
}
