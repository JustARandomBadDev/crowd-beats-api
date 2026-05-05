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

	client := &Client{conn: conn, send: make(chan []byte, 32)}
	hub := h.registry.hub(roomID)
	hub.register <- client

	go func() {
		defer func() {
			hub.unregister <- client
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	go func() {
		for msg := range client.send {
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		}
	}()

	h.registry.Broadcast(usecase.LiveEvent{
		Name:      "room_joined",
		RoomID:    roomID,
		Timestamp: h.usecases.Now(),
		Payload: map[string]any{
			"session_id": current.ID,
			"nickname":   current.Nickname,
		},
	})
}
