package handlers

import (
	"net/http"

	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/infra/http/middleware"

	"github.com/google/uuid"
)

func (h *Handler) CreateTrack(w http.ResponseWriter, r *http.Request) {
	current := middleware.SessionFromContext(r.Context())
	roomID, err := middleware.ParseID(r.PathValue("roomID"), "room")
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	var req struct {
		SpotifyTrackID string `json:"spotify_track_id"`
	}
	if err := middleware.DecodeJSON(r, &req, false); err != nil {
		middleware.WriteError(w, err)
		return
	}
	response, status, err := h.Usecases.ProposeTrack(r.Context(), roomID, current, req.SpotifyTrackID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, status, dto.ProposeTrackFromResult(response))
}

func (h *Handler) DeleteTrack(w http.ResponseWriter, r *http.Request) {
	roomID, err := middleware.ParseID(r.PathValue("roomID"), "room")
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	roomTrackID, err := middleware.ParseID(r.PathValue("roomTrackID"), "room track")
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	if err := h.Usecases.DeleteTrack(r.Context(), roomID, roomTrackID); err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, dto.DeleteTrackResponse{Deleted: true, RoomTrackID: roomTrackID})
}

func (h *Handler) SkipTrack(w http.ResponseWriter, r *http.Request) {
	h.setTrackLifecycle(w, r, track.StatusSkipped, "track_skipped", "skipped_at")
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
	middleware.WriteJSON(w, http.StatusOK, dto.TrackActionResponse{Updated: true, RoomTrackID: roomTrackID, Status: track.StatusPlaying})
}

func (h *Handler) MarkPlayed(w http.ResponseWriter, r *http.Request) {
	h.setTrackLifecycle(w, r, track.StatusPlayed, "track_played", "played_at")
}

func (h *Handler) setTrackLifecycle(w http.ResponseWriter, r *http.Request, status, eventName, tsColumn string) {
	roomID, roomTrackID, err := roomAndTrackID(r)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	if err := h.Usecases.SetTrackStatus(r.Context(), roomID, roomTrackID, status, eventName, tsColumn); err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, dto.TrackActionResponse{Updated: true, RoomTrackID: roomTrackID, Status: status})
}

func roomAndTrackID(r *http.Request) (uuid.UUID, uuid.UUID, error) {
	roomID, err := middleware.ParseID(r.PathValue("roomID"), "room")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	roomTrackID, err := middleware.ParseID(r.PathValue("roomTrackID"), "room track")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return roomID, roomTrackID, nil
}
