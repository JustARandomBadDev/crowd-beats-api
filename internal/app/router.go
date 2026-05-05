package app

import (
	"net/http"

	"crowdbeats/internal/infra/http/handlers"
	"crowdbeats/internal/infra/http/middleware"
	"crowdbeats/internal/infra/ws"
	"crowdbeats/internal/usecase"
)

func NewRouter(cfg Config, usecases *usecase.Services, registry *ws.Registry) http.Handler {
	h := handlers.New(usecases)
	wsHandler := ws.NewHandler(registry, usecases)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", h.Live)
	mux.HandleFunc("GET /health/ready", h.Ready)
	mux.Handle("GET /ws", wsHandler)

	mux.HandleFunc("POST /api/v1/rooms/join-by-qr", h.JoinByQR)
	mux.HandleFunc("GET  /api/v1/rooms/{roomID}", h.GetRoom)
	mux.HandleFunc("GET  /api/v1/rooms/{roomID}/queue", h.GetQueue)
	mux.HandleFunc("GET  /api/v1/rooms/{roomID}/stats", h.GetRoomStats)

	mux.Handle("GET  /api/v1/spotify/search", middleware.RequireSession(usecases, http.HandlerFunc(h.SearchSpotify)))
	mux.Handle("POST /api/v1/rooms/{roomID}/tracks", middleware.RequireSession(usecases, http.HandlerFunc(h.CreateTrack)))
	mux.Handle("POST /api/v1/rooms/{roomID}/votes", middleware.RequireSession(usecases, http.HandlerFunc(h.AddVote)))
	mux.Handle("GET  /api/v1/sessions/me", middleware.RequireSession(usecases, http.HandlerFunc(h.SessionMe)))
	mux.Handle("POST /api/v1/sessions/heartbeat", middleware.RequireSession(usecases, http.HandlerFunc(h.Heartbeat)))
	mux.Handle("POST /api/v1/sessions/leave", middleware.RequireSession(usecases, http.HandlerFunc(h.Leave)))

	mux.HandleFunc("POST /api/v1/manager/rooms", h.CreateRoom)
	mux.Handle("POST     /api/v1/manager/rooms/{roomID}/qr-codes", middleware.RequireManager(usecases, http.HandlerFunc(h.CreateQRCode)))
	mux.Handle("PATCH    /api/v1/manager/rooms/{roomID}", middleware.RequireManager(usecases, http.HandlerFunc(h.PatchRoom)))
	mux.Handle("GET      /api/v1/manager/rooms/{roomID}/stats", middleware.RequireManager(usecases, http.HandlerFunc(h.ManagerStats)))
	mux.Handle("DELETE   /api/v1/rooms/{roomID}/tracks/{roomTrackID}", middleware.RequireManager(usecases, http.HandlerFunc(h.DeleteTrack)))
	mux.Handle("POST     /api/v1/rooms/{roomID}/tracks/{roomTrackID}/skip", middleware.RequireManager(usecases, http.HandlerFunc(h.SkipTrack)))
	mux.Handle("POST     /api/v1/rooms/{roomID}/tracks/{roomTrackID}/mark-playing", middleware.RequireManager(usecases, http.HandlerFunc(h.MarkPlaying)))
	mux.Handle("POST     /api/v1/rooms/{roomID}/tracks/{roomTrackID}/mark-played", middleware.RequireManager(usecases, http.HandlerFunc(h.MarkPlayed)))

	return middleware.CORS(cfg.AllowedOrigins)(middleware.JSON(mux))
}
