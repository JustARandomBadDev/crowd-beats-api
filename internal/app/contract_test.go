package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/spotify"
	"crowdbeats/internal/infra/ws"
	"crowdbeats/internal/testutil"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func serveContractRequest(h *testutil.Harness, method, path, body, bearer string) *httptest.ResponseRecorder {
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), nil)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func contractData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	return envelope.Data
}

func assertNoInternalFields(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, value := range []string{"managersecrethash", "manager_secret_hash", "sessiontokenhash", "session_token_hash", "rawpayload", "raw_payload", "fifoorder", "fifo_order", "private-hash-value", "private-spotify-payload"} {
		require.NotContains(t, lower, value)
	}
}

func TestPublicRoomAndSessionResponsesExcludeHashes(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	hash := "private-hash-value"
	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, Name: "Bar", ManagerSecretHash: &hash, QueueLimit: 20}, nil
	}
	h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
		return session.Session{ID: uuid.New(), RoomID: roomID, Nickname: "Alex", Status: session.StatusActive, SessionTokenHash: hash}, nil
	}

	roomRec := serveContractRequest(h, http.MethodGet, "/api/v1/rooms/"+roomID.String(), "", "")
	require.Equal(t, http.StatusOK, roomRec.Code)
	assertNoInternalFields(t, roomRec.Body.String())
	roomData := contractData(t, roomRec)
	require.Equal(t, "Bar", roomData["name"])
	require.Contains(t, roomData, "queue_limit")
	require.NotContains(t, roomData, "Name")

	sessionRec := serveContractRequest(h, http.MethodGet, "/api/v1/sessions/me", "", "bearer")
	require.Equal(t, http.StatusOK, sessionRec.Code)
	assertNoInternalFields(t, sessionRec.Body.String())
	sessionData := contractData(t, sessionRec)["session"].(map[string]any)
	require.Equal(t, roomID.String(), sessionData["room_id"])
	require.NotContains(t, sessionData, "token")
}

func TestJoinResponseContainsOnlyAcquiredToken(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	hash := "private-hash-value"
	h.Repos.RoomRepo.GetByQRCodeFn = func(context.Context, string) (room.Room, error) {
		return room.Room{ID: roomID, Name: "Bar", Status: room.StatusActive, ManagerSecretHash: &hash}, nil
	}
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, Name: "Bar", Status: room.StatusActive, ManagerSecretHash: &hash}, nil
	}
	h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
		return session.Session{}, pgx.ErrNoRows
	}
	h.Repos.SessionRepo.CreateFn = func(_ context.Context, input session.CreateInput) (session.Session, error) {
		return session.Session{ID: uuid.New(), RoomID: roomID, Nickname: input.Nickname, Role: input.Role, Status: session.StatusActive, SessionTokenHash: hash}, nil
	}
	rec := serveContractRequest(h, http.MethodPost, "/api/v1/rooms/join-by-qr", `{"qr_code":"qr_test","nickname":"Alex"}`, "")
	require.Equal(t, http.StatusOK, rec.Code)
	assertNoInternalFields(t, rec.Body.String())
	data := contractData(t, rec)
	require.Equal(t, "test-token", data["session"].(map[string]any)["token"])
	require.Equal(t, roomID.String(), data["room"].(map[string]any)["id"])
	require.Contains(t, data["ws"].(map[string]any)["url"], roomID.String())
}

func TestQueueDistinguishesExistingEmptyRoomFromMissingRoom(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	h.Repos.RoomRepo.GetByIDFn = func(_ context.Context, id uuid.UUID) (room.Room, error) {
		if id == roomID {
			return room.Room{ID: id}, nil
		}
		return room.Room{}, pgx.ErrNoRows
	}
	h.Repos.QueueRepo.LoadFn = func(context.Context, uuid.UUID) ([]queue.Item, *time.Time, error) {
		return nil, nil, nil
	}
	path := "/api/v1/rooms/" + roomID.String() + "/queue"
	rec := serveContractRequest(h, http.MethodGet, path, "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, []any{}, contractData(t, rec)["items"])
	require.Nil(t, contractData(t, rec)["now_playing"])
	require.Nil(t, contractData(t, rec)["updated_at"])

	missing := serveContractRequest(h, http.MethodGet, "/api/v1/rooms/"+uuid.NewString()+"/queue", "", "")
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.Contains(t, missing.Body.String(), `"code":"NOT_FOUND"`)
}

func TestQueueAndSpotifySearchHideInternalTrackFields(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID}, nil
	}
	h.Repos.QueueRepo.LoadFn = func(context.Context, uuid.UUID) ([]queue.Item, *time.Time, error) {
		return []queue.Item{{RoomTrackID: uuid.New(), Position: 1, FIFOOrder: 99, SpotifyTrackID: "spotify-id", Title: "Song", VoteCount: 2}}, nil, nil
	}
	h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
		return session.Session{RoomID: roomID, Status: session.StatusActive}, nil
	}
	h.Repos.SpotifyProvider.SearchTracksFn = func(context.Context, string) ([]spotify.Track, error) {
		return []spotify.Track{{SpotifyTrackID: "spotify-id", Title: "Song", RawPayload: json.RawMessage(`{"private":"private-spotify-payload"}`)}}, nil
	}

	queueRec := serveContractRequest(h, http.MethodGet, "/api/v1/rooms/"+roomID.String()+"/queue", "", "")
	require.Equal(t, http.StatusOK, queueRec.Code)
	assertNoInternalFields(t, queueRec.Body.String())
	item := contractData(t, queueRec)["items"].([]any)[0].(map[string]any)
	require.Equal(t, "Song", item["track"].(map[string]any)["title"])
	require.Contains(t, item, "vote_count")

	searchRec := serveContractRequest(h, http.MethodGet, "/api/v1/spotify/search?q=Song", "", "bearer")
	require.Equal(t, http.StatusOK, searchRec.Code)
	assertNoInternalFields(t, searchRec.Body.String())
	searchTrack := contractData(t, searchRec)["items"].([]any)[0].(map[string]any)
	require.Equal(t, "spotify-id", searchTrack["spotify_track_id"])
}

func TestRESTClientErrorsAreStable(t *testing.T) {
	h := testutil.NewHarness()
	invalidID := serveContractRequest(h, http.MethodGet, "/api/v1/rooms/invalid", "", "")
	require.Equal(t, http.StatusBadRequest, invalidID.Code)
	require.Contains(t, invalidID.Body.String(), `"code":"INVALID_ID"`)

	invalidJSON := serveContractRequest(h, http.MethodPost, "/api/v1/manager/rooms", `{`, "")
	require.Equal(t, http.StatusBadRequest, invalidJSON.Code)
	require.Contains(t, invalidJSON.Body.String(), `"code":"INVALID_JSON"`)

	called := false
	h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
		return session.Session{Status: session.StatusActive}, nil
	}
	h.Repos.SpotifyProvider.SearchTracksFn = func(context.Context, string) ([]spotify.Track, error) {
		called = true
		return nil, nil
	}
	emptySearch := serveContractRequest(h, http.MethodGet, "/api/v1/spotify/search?q=%20", "", "bearer")
	require.Equal(t, http.StatusBadRequest, emptySearch.Code)
	require.Contains(t, emptySearch.Body.String(), `"code":"VALIDATION_ERROR"`)
	require.False(t, called)

	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{}, errors.New("private database password detail")
	}
	internal := serveContractRequest(h, http.MethodGet, "/api/v1/rooms/"+uuid.NewString(), "", "")
	require.Equal(t, http.StatusInternalServerError, internal.Code)
	require.Contains(t, internal.Body.String(), `"code":"INTERNAL_ERROR"`)
	require.NotContains(t, internal.Body.String(), "private database password detail")

	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{}, &pgconn.PgError{Code: "23505", Message: "private sql constraint detail", ConstraintName: "uq_private"}
	}
	pg := serveContractRequest(h, http.MethodGet, "/api/v1/rooms/"+uuid.NewString(), "", "")
	require.Equal(t, http.StatusInternalServerError, pg.Code)
	require.Contains(t, pg.Body.String(), `"code":"INTERNAL_ERROR"`)
	require.NotContains(t, pg.Body.String(), "private sql constraint detail")
	require.NotContains(t, pg.Body.String(), "23505")
}

func TestCreateRoomRejectsExplicitInvalidValues(t *testing.T) {
	h := testutil.NewHarness()
	for _, body := range []string{
		`{"name":"Bar","recalc_interval_seconds":0}`,
		`{"name":"Bar","recalc_interval_seconds":-5}`,
		`{"name":"   "}`,
		`{"name":"Bar","status":"invalid"}`,
	} {
		rec := serveContractRequest(h, http.MethodPost, "/api/v1/manager/rooms", body, "")
		require.Equal(t, http.StatusBadRequest, rec.Code)
		require.Contains(t, rec.Body.String(), `"code":"VALIDATION_ERROR"`)
	}
}

func TestCreateQRCodeForMissingRoomReturnsNotFound(t *testing.T) {
	h := testutil.NewHarness()
	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{}, pgx.ErrNoRows
	}
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/manager/rooms/"+uuid.NewString()+"/qr-codes", strings.NewReader(`{}`))
	req.Header.Set("X-Manager-Secret", "some-secret")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"NOT_FOUND"`)
}

func TestSpotifyProviderFailureHasPublicCode(t *testing.T) {
	h := testutil.NewHarness()
	h.Repos.SessionRepo.GetByTokenHashFn = func(context.Context, string) (session.Session, error) {
		return session.Session{Status: session.StatusActive}, nil
	}
	h.Repos.SpotifyProvider.SearchTracksFn = func(context.Context, string) ([]spotify.Track, error) {
		return nil, &spotify.ProviderError{StatusCode: http.StatusTooManyRequests, Cause: errors.New("private Spotify URL and body")}
	}
	rec := serveContractRequest(h, http.MethodGet, "/api/v1/spotify/search?q=Song", "", "bearer")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"SPOTIFY_RATE_LIMITED"`)
	require.NotContains(t, rec.Body.String(), "private Spotify URL")
}

func TestStatsOnlyAvailableOnManagerRoute(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	public := serveContractRequest(h, http.MethodGet, "/api/v1/rooms/"+roomID.String()+"/stats", "", "")
	require.Equal(t, http.StatusNotFound, public.Code)

	secretHash := "hashed:manager-token"
	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, ManagerSecretHash: &secretHash}, nil
	}
	h.Repos.RoomRepo.LoadStatsFn = func(context.Context, uuid.UUID) (room.Stats, error) {
		return room.Stats{ActiveUsers: 2, TopTracks: []room.TopTrack{{Title: "Song", Votes: 3}}}, nil
	}
	router := NewRouter(Config{AllowedOrigins: "*"}, h.Services, ws.NewRegistry(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/manager/rooms/"+roomID.String()+"/stats", nil)
	req.Header.Set("X-Manager-Secret", "manager-token")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assertNoInternalFields(t, rec.Body.String())
	data := contractData(t, rec)
	require.Equal(t, float64(2), data["active_users"])
	require.Equal(t, "Song", data["top_tracks"].([]any)[0].(map[string]any)["title"])
}
