package handlers

import (
	"net/http"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/infra/http/middleware"
	"crowdbeats/internal/usecase"
)

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name                  string  `json:"name"`
		Slug                  *string `json:"slug"`
		Status                *string `json:"status"`
		QueueLimit            *int    `json:"queue_limit"`
		MaxVotesPerUser       *int    `json:"max_votes_per_user"`
		QRTTLSeconds          *int    `json:"qr_ttl_seconds"`
		RecalcIntervalSeconds *int    `json:"recalc_interval_seconds"`
	}
	if err := middleware.DecodeJSON(r, &req, true); err != nil {
		middleware.WriteError(w, err)
		return
	}
	created, secret, err := h.Usecases.CreateRoom(r.Context(), usecase.CreateRoomInput{
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
	middleware.WriteJSON(w, http.StatusCreated, dto.CreateRoomResponse{
		Room: dto.RoomFromDomain(created), ManagerSecret: secret,
	})
}

func (h *Handler) CreateQRCode(w http.ResponseWriter, r *http.Request) {
	currentRoom := middleware.RoomFromContext(r.Context())
	var req struct {
		ExpiresInSeconds *int `json:"expires_in_seconds"`
	}
	if err := middleware.DecodeJSON(r, &req, true); err != nil {
		middleware.WriteError(w, err)
		return
	}
	qr, err := h.Usecases.CreateQRCode(r.Context(), currentRoom.ID, req.ExpiresInSeconds)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusCreated, dto.QRCodeFromDomain(qr))
}

func (h *Handler) PatchRoom(w http.ResponseWriter, r *http.Request) {
	currentRoom := middleware.RoomFromContext(r.Context())
	var req struct {
		Status          *string `json:"status"`
		QueueLimit      *int    `json:"queue_limit"`
		MaxVotesPerUser *int    `json:"max_votes_per_user"`
	}
	if err := middleware.DecodeJSON(r, &req, true); err != nil {
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
	middleware.WriteJSON(w, http.StatusOK, dto.RoomContainerResponse{Room: dto.RoomFromDomain(updated)})
}

func (h *Handler) ManagerStats(w http.ResponseWriter, r *http.Request) {
	currentRoom := middleware.RoomFromContext(r.Context())
	stats, err := h.Usecases.GetRoomStats(r.Context(), currentRoom.ID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, dto.RoomStatsFromDomain(stats))
}
