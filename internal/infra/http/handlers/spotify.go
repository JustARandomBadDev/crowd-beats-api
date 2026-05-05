package handlers

import (
	"net/http"
	"strings"

	"crowdbeats/internal/infra/http/middleware"
)

func (h *Handler) SearchSpotify(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	items, err := h.Usecases.SearchSpotify(r.Context(), query)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	middleware.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}
