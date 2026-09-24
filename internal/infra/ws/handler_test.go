package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/testutil"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type fakeWSConnection struct {
	readLimit      int64
	readDeadlines  []time.Time
	writeDeadlines []time.Time
	pong           func(string) error
	writes         []int
}

func (f *fakeWSConnection) SetReadLimit(limit int64) { f.readLimit = limit }
func (f *fakeWSConnection) SetReadDeadline(deadline time.Time) error {
	f.readDeadlines = append(f.readDeadlines, deadline)
	return nil
}
func (f *fakeWSConnection) SetPongHandler(handler func(string) error) { f.pong = handler }
func (f *fakeWSConnection) SetWriteDeadline(deadline time.Time) error {
	f.writeDeadlines = append(f.writeDeadlines, deadline)
	return nil
}
func (f *fakeWSConnection) WriteMessage(messageType int, _ []byte) error {
	f.writes = append(f.writes, messageType)
	return nil
}

func TestHandshakeRejectsMissingTokenAndWrongRoomBeforeUpgrade(t *testing.T) {
	h := testutil.NewHarness()
	roomID, sessionID := uuid.New(), uuid.New()
	h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
		return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
	}
	handler := NewHandler(NewRegistry(), h.Services)
	request := httptest.NewRequest(http.MethodGet, "/ws?room_id="+roomID.String(), nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusUnauthorized, response.Code)
	require.Equal(t, "unauthorized\n", response.Body.String())

	request = httptest.NewRequest(http.MethodGet, "/ws?room_id="+uuid.NewString(), nil)
	request.Header.Set("Authorization", "Bearer session-token")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusForbidden, response.Code)
}

func TestHandshakeKeepsAuthenticationErrorsPrivate(t *testing.T) {
	for _, tc := range []struct {
		name       string
		repoError  error
		wantStatus int
		wantBody   string
	}{
		{name: "unknown session", repoError: pgx.ErrNoRows, wantStatus: http.StatusUnauthorized, wantBody: "unauthorized\n"},
		{name: "database failure", repoError: errors.New("postgres connection refused: secret detail"), wantStatus: http.StatusInternalServerError, wantBody: "internal server error\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testutil.NewHarness()
			h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
				return session.Session{}, tc.repoError
			}
			handler := NewHandler(NewRegistry(), h.Services)
			request := httptest.NewRequest(http.MethodGet, "/ws?room_id="+uuid.NewString(), nil)
			request.Header.Set("Authorization", "Bearer session-token")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, tc.wantStatus, response.Code)
			require.Equal(t, tc.wantBody, response.Body.String())
		})
	}
}

func TestConnectionPolicyBounds(t *testing.T) {
	require.Positive(t, writeWait)
	require.Less(t, pingEvery, pongWait)
	require.Greater(t, maxMessage, 0)
	require.LessOrEqual(t, maxMessage, 4096)
}

func TestWebSocketDeadlinesAndPongRenewal(t *testing.T) {
	conn := &fakeWSConnection{}
	start := time.Now()
	require.NoError(t, configureRead(conn))
	require.Equal(t, int64(maxMessage), conn.readLimit)
	require.Len(t, conn.readDeadlines, 1)
	require.WithinDuration(t, start.Add(pongWait), conn.readDeadlines[0], time.Second)
	require.NotNil(t, conn.pong)
	require.NoError(t, conn.pong("heartbeat"))
	require.Len(t, conn.readDeadlines, 2)
	require.False(t, conn.readDeadlines[1].Before(conn.readDeadlines[0]))

	require.NoError(t, writeWithDeadline(conn, websocket.TextMessage, []byte("event")))
	require.NoError(t, writeWithDeadline(conn, websocket.PingMessage, nil))
	require.Equal(t, []int{websocket.TextMessage, websocket.PingMessage}, conn.writes)
	require.Len(t, conn.writeDeadlines, 2)
	for _, deadline := range conn.writeDeadlines {
		require.WithinDuration(t, start.Add(writeWait), deadline, time.Second)
	}
}
