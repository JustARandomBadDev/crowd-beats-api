package middleware

import (
	"context"
	"net/http"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/platform/auth"
	"crowdbeats/internal/usecase"
)

type contextKey string

const (
	sessionKey contextKey = "session"
	roomKey    contextKey = "room"
)

func RequireSession(usecases *usecase.Services, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current, err := usecases.AuthenticateSession(r.Context(), auth.BearerToken(r.Header.Get("Authorization")))
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, current)))
	})
}

func RequireManager(usecases *usecase.Services, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roomID, err := ParseID(r.PathValue("roomID"), "room")
		if err != nil {
			writeError(w, err)
			return
		}
		current, err := usecases.AuthenticateManager(r.Context(), roomID, r.Header.Get("X-Manager-Secret"))
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), roomKey, current)))
	})
}

func SessionFromContext(ctx context.Context) session.Session {
	value, _ := ctx.Value(sessionKey).(session.Session)
	return value
}

func RoomFromContext(ctx context.Context) room.Room {
	value, _ := ctx.Value(roomKey).(room.Room)
	return value
}
