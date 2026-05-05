package handlers

import (
	"encoding/json"
	"net/http"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/infra/http/middleware"
)

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                  string  `json:"name"`
		Slug                  *string `json:"slug"`
		Status                string  `json:"status"`
		QueueLimit            int     `json:"queue_limit"`
		MaxVotesPerUser       int     `json:"max_votes_per_user"`
		QRTTLSeconds          int     `json:"qr_ttl_seconds"`
		RecalcIntervalSeconds int     `json:"recalc_interval_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		middleware.WriteError(w, err)
		return
	}
	if req.Status == "" {
		req.Status = room.StatusDraft
	}
	if req.QueueLimit == 0 {
		req.QueueLimit = 20
	}
	if req.MaxVotesPerUser == 0 {
		req.MaxVotesPerUser = 5
	}
	if req.QRTTLSeconds == 0 {
		req.QRTTLSeconds = 14400
	}
	created, secret, err := h.Usecases.CreateRoom(r.Context(), room.CreateInput{
		Name:                  req.Name,
		Slug:                  req.Slug,
		Status:                req.Status,
		QueueLimit:            req.QueueLimit,
		MaxVotesPerUser:       req.MaxVotesPerUser,
		QRTTLSeconds:          req.QRTTLSeconds,
		RecalcIntervalSeconds: req.RecalcIntervalSeconds,
	})
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, map[string]any{
		"room":           created,
		"manager_secret": secret,
	})
}

func (h *Handler) CreateQRCode(w http.ResponseWriter, r *http.Request) {
	currentRoom := middleware.RoomFromContext(r.Context())
	var req struct {
		ExpiresInSeconds int `json:"expires_in_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		middleware.WriteError(w, err)
		return
	}
	qr, err := h.Usecases.CreateQRCode(r.Context(), currentRoom.ID, req.ExpiresInSeconds, currentRoom.QRTTLSeconds)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, qr)
}

func (h *Handler) PatchRoom(w http.ResponseWriter, r *http.Request) {
	currentRoom := middleware.RoomFromContext(r.Context())
	var req struct {
		Status          *string `json:"status"`
		QueueLimit      *int    `json:"queue_limit"`
		MaxVotesPerUser *int    `json:"max_votes_per_user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		middleware.WriteError(w, err)
		return
	}
	updated, err := h.Usecases.PatchRoom(r.Context(), currentRoom.ID, room.Patch{
		Status:          req.Status,
		QueueLimit:      req.QueueLimit,
		MaxVotesPerUser: req.MaxVotesPerUser,
	})
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"room": updated})
}

func (h *Handler) ManagerStats(w http.ResponseWriter, r *http.Request) {
	currentRoom := middleware.RoomFromContext(r.Context())
	stats, err := h.Usecases.GetRoomStats(r.Context(), currentRoom.ID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, stats)
}
