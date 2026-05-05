package handlers

import (
	"encoding/json"
	"net/http"

	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/infra/http/middleware"

	"github.com/google/uuid"
)

func (h *Handler) CreateTrack(w http.ResponseWriter, r *http.Request) {
	current := middleware.SessionFromContext(r.Context())
	roomID, err := uuid.Parse(r.PathValue("roomID"))
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	var req struct {
		SpotifyTrackID string `json:"spotify_track_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, err)
		return
	}
	response, status, err := h.Usecases.ProposeTrack(r.Context(), roomID, current, req.SpotifyTrackID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, status, response)
}

func (h *Handler) DeleteTrack(w http.ResponseWriter, r *http.Request) {
	roomID, err := uuid.Parse(r.PathValue("roomID"))
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	roomTrackID, err := uuid.Parse(r.PathValue("roomTrackID"))
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	if err := h.Usecases.DeleteTrack(r.Context(), roomID, roomTrackID); err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"deleted": true, "room_track_id": roomTrackID})
}

func (h *Handler) SkipTrack(w http.ResponseWriter, r *http.Request) {
	h.setTrackLifecycle(w, r, track.StatusSkipped, "track_skipped", "skipped_at", true)
}

func (h *Handler) MarkPlaying(w http.ResponseWriter, r *http.Request) {
	roomID, roomTrackID, err := roomAndTrackID(r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	if err := h.Usecases.SetTrackPlaying(r.Context(), roomID, roomTrackID); err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"updated": true, "room_track_id": roomTrackID, "status": track.StatusPlaying})
}

func (h *Handler) MarkPlayed(w http.ResponseWriter, r *http.Request) {
	h.setTrackLifecycle(w, r, track.StatusPlayed, "track_played", "played_at", true)
}

func (h *Handler) setTrackLifecycle(w http.ResponseWriter, r *http.Request, status, eventName, tsColumn string, recalcNow bool) {
	roomID, roomTrackID, err := roomAndTrackID(r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	if err := h.Usecases.SetTrackStatus(r.Context(), roomID, roomTrackID, status, eventName, tsColumn, recalcNow); err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"updated": true, "room_track_id": roomTrackID, "status": status})
}

func roomAndTrackID(r *http.Request) (uuid.UUID, uuid.UUID, error) {
	roomID, err := uuid.Parse(r.PathValue("roomID"))
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	roomTrackID, err := uuid.Parse(r.PathValue("roomTrackID"))
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return roomID, roomTrackID, nil
}
