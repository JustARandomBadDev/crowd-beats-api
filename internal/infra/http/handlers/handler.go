package handlers

import "crowdbeats/internal/usecase"

type Handler struct {
	Usecases *usecase.Services
}

func New(usecases *usecase.Services) *Handler {
	return &Handler{Usecases: usecases}
}
