package handlers

import (
	"encoding/json"
	"net/http"

	"crowdbeats/internal/infra/http/middleware"

	"github.com/google/uuid"
)

func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomID, err := uuid.Parse(r.PathValue("roomID"))
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	current, err := h.Usecases.GetRoom(r.Context(), roomID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, current)
}

func (h *Handler) JoinByQR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QRCode       string `json:"qr_code"`
		Nickname     string `json:"nickname"`
		SessionToken string `json:"session_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, err)
		return
	}
	result, err := h.Usecases.JoinByQRCode(r.Context(), req.QRCode, req.Nickname, req.SessionToken)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{
		"room":    result.Room,
		"session": result.Session,
		"ws": map[string]any{
			"url": result.WebSocketURL,
		},
	})
}

func (h *Handler) GetQueue(w http.ResponseWriter, r *http.Request) {
	roomID, err := uuid.Parse(r.PathValue("roomID"))
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	items, updatedAt, err := h.Usecases.GetQueue(r.Context(), roomID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{
		"items":      items,
		"updated_at": updatedAt,
	})
}

func (h *Handler) GetRoomStats(w http.ResponseWriter, r *http.Request) {
	roomID, err := uuid.Parse(r.PathValue("roomID"))
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	stats, err := h.Usecases.GetRoomStats(r.Context(), roomID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, stats)
}
