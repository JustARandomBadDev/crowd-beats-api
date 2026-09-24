package middleware

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/pkg/apierror"

	"github.com/jackc/pgx/v5"
)

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dto.Envelope{
		Data:  data,
		Error: nil,
		Meta:  map[string]any{},
	})
}

func WriteError(w http.ResponseWriter, err error) {
	writeError(w, err)
}

func writeError(w http.ResponseWriter, err error) {
	var apiErr apierror.Error
	if errors.As(err, &apiErr) {
		w.WriteHeader(apiErr.Status)
		_ = json.NewEncoder(w).Encode(dto.Envelope{
			Data: nil,
			Error: &dto.APIError{
				Code:    apiErr.Code,
				Message: apiErr.Message,
			},
			Meta: map[string]any{},
		})
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, apierror.New("NOT_FOUND", "resource not found", http.StatusNotFound))
		return
	}
	log.Printf("HTTP internal error: %v", err)
	writeError(w, apierror.New("INTERNAL_ERROR", "an internal error occurred", http.StatusInternalServerError))
}
