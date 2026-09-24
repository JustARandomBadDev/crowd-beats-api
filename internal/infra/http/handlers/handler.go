package handlers

import (
	"context"

	"crowdbeats/internal/usecase"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

type Handler struct {
	Usecases *usecase.Services
	Database DatabasePinger
}

func New(usecases *usecase.Services, database DatabasePinger) *Handler {
	return &Handler{Usecases: usecases, Database: database}
}
