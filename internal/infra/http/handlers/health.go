package handlers

import (
	"context"
	"net/http"
	"time"

	"crowdbeats/internal/infra/http/middleware"
)

func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	writeHealth(w, http.StatusOK, "ok")
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	if h.Database == nil {
		writeHealth(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
	defer cancel()
	if err := h.Database.Ping(ctx); err != nil {
		writeHealth(w, http.StatusServiceUnavailable, "not_ready")
		return
	}
	writeHealth(w, http.StatusOK, "ready")
}

func writeHealth(w http.ResponseWriter, status int, state string) {
	middleware.WriteJSON(w, status, map[string]string{"status": state})
}
