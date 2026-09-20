package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
)

func DecodeJSON(r *http.Request, target any, optional bool) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(target); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return nil
		}
		return apierror.New("INVALID_JSON", "invalid JSON body", http.StatusBadRequest)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return apierror.New("INVALID_JSON", "invalid JSON body", http.StatusBadRequest)
	}
	return nil
}

func ParseID(raw, label string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, apierror.New("INVALID_ID", "invalid "+label+" ID", http.StatusBadRequest)
	}
	return id, nil
}
