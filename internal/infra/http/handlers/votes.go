package handlers

import (
	"encoding/json"
	"net/http"

	"crowdbeats/internal/infra/http/middleware"

	"github.com/google/uuid"
)

func (h *Handler) AddVote(w http.ResponseWriter, r *http.Request) {
	current := middleware.SessionFromContext(r.Context())
	roomID, err := uuid.Parse(r.PathValue("roomID"))
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	var req struct {
		RoomTrackID uuid.UUID `json:"room_track_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, err)
		return
	}
	response, err := h.Usecases.AddVote(r.Context(), roomID, current, req.RoomTrackID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, response)
}
