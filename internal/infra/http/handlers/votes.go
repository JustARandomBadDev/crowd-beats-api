package handlers

import (
	"net/http"

	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/infra/http/middleware"

	"github.com/google/uuid"
)

func (h *Handler) AddVote(w http.ResponseWriter, r *http.Request) {
	current := middleware.SessionFromContext(r.Context())
	roomID, err := middleware.ParseID(r.PathValue("roomID"), "room")
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	var req struct {
		RoomTrackID uuid.UUID `json:"room_track_id"`
	}
	if err := middleware.DecodeJSON(r, &req, false); err != nil {
		middleware.WriteError(w, err)
		return
	}
	response, err := h.Usecases.AddVote(r.Context(), roomID, current, req.RoomTrackID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, dto.VoteFromResult(response))
}
