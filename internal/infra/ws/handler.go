package ws

import (
	"net/http"

	"crowdbeats/internal/platform/auth"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Handler struct {
	registry *Registry
	usecases *usecase.Services
	upgrader websocket.Upgrader
}

func NewHandler(registry *Registry, usecases *usecase.Services) *Handler {
	return &Handler{
		registry: registry,
		usecases: usecases,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	current, err := h.usecases.AuthenticateSession(r.Context(), auth.BearerToken(r.Header.Get("Authorization")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	roomID, err := uuid.Parse(r.URL.Query().Get("room_id"))
	if err != nil || roomID == uuid.Nil {
		http.Error(w, "room_id is required", http.StatusBadRequest)
		return
	}
	if current.RoomID != roomID {
		http.Error(w, "session is not in room", http.StatusForbidden)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	client := newClient(conn, roomID, current.ID, 32)
	h.registry.RegisterPending(roomID, client)
	// A room switch may commit between the first auth check and registration.
	// Pending clients receive no broadcasts until this second check succeeds.
	confirmed, err := h.usecases.AuthenticateSession(r.Context(), auth.BearerToken(r.Header.Get("Authorization")))
	if err != nil || confirmed.ID != current.ID || confirmed.RoomID != roomID {
		h.registry.Unregister(roomID, client)
		return
	}
	if !h.registry.Activate(roomID, client) {
		return
	}
	go client.readPump(h.registry)
	go client.writePump(h.registry)
}
