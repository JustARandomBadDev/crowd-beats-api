package handlers

import (
	"net/http"
	"strings"

	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/infra/http/middleware"
	"crowdbeats/pkg/apierror"
)

func (h *Handler) SearchSpotify(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		middleware.WriteError(w, apierror.New("VALIDATION_ERROR", "search query is required", http.StatusBadRequest))
		return
	}
	items, err := h.Usecases.SearchSpotify(r.Context(), query)
	if err != nil {
		middleware.WriteError(w, err)
		return
	}
	response := dto.SpotifySearchResponse{Items: make([]dto.SpotifyTrackResponse, 0, len(items))}
	for _, item := range items {
		response.Items = append(response.Items, dto.SpotifyTrackFromDomain(item))
	}
	middleware.WriteJSON(w, http.StatusOK, response)
}
