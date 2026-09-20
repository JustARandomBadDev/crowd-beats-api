package handlers

import (
	"net/http"

	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/infra/http/middleware"
)

func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	roomID, err := middleware.ParseID(r.PathValue("roomID"), "room")
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	current, err := h.Usecases.GetRoom(r.Context(), roomID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, dto.RoomFromDomain(current))
}

func (h *Handler) JoinByQR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		QRCode       string `json:"qr_code"`
		Nickname     string `json:"nickname"`
		SessionToken string `json:"session_token"`
	}
	if err := middleware.DecodeJSON(r, &req, false); err != nil {
		middleware.WriteError(w, err)
		return
	}
	result, err := h.Usecases.JoinByQRCode(r.Context(), req.QRCode, req.Nickname, req.SessionToken)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, dto.JoinRoomResponse{
		Room: dto.RoomFromDomain(result.Room),
		Session: dto.JoinSessionResponse{
			ID: result.Session.ID, Nickname: result.Session.Nickname,
			Role: result.Session.Role, Token: result.Token,
		},
		WS: dto.WebSocketResponse{URL: result.WebSocketURL},
	})
}

func (h *Handler) GetQueue(w http.ResponseWriter, r *http.Request) {
	roomID, err := middleware.ParseID(r.PathValue("roomID"), "room")
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	if _, err := h.Usecases.GetRoom(r.Context(), roomID); err != nil {
		middleware.WriteError(w, err)
		return
	}
	items, updatedAt, err := h.Usecases.GetQueue(r.Context(), roomID)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, dto.QueueSnapshotFromDomain(items, updatedAt))
}
