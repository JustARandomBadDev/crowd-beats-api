package handlers

import (
	"net/http"

	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/infra/http/middleware"

	"github.com/google/uuid"
)

func (h *Handler) SessionMe(w http.ResponseWriter, r *http.Request) {
	middleware.WriteJSON(w, http.StatusOK, dto.SessionContainerResponse{
		Session: dto.SessionFromDomain(middleware.SessionFromContext(r.Context())),
	})
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	current := middleware.SessionFromContext(r.Context())
	var req struct {
		RoomID uuid.UUID `json:"room_id"`
	}
	if err := middleware.DecodeJSON(r, &req, true); err != nil {
		middleware.WriteError(w, err)
		return
	}
	if err := h.Usecases.Heartbeat(r.Context(), current, req.RoomID); err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	current := middleware.SessionFromContext(r.Context())
	if err := h.Usecases.Leave(r.Context(), current); err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"left": true})
}
