package usecase_test

import (
	"context"
	"net/http"
	"testing"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/testutil"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestAddVoteRejectsAlreadyVotedForTrack(t *testing.T) {
	h := testutil.NewHarness()
	roomID := uuid.New()
	sessionID := uuid.New()
	roomTrackID := uuid.New()

	h.Repos.RoomRepo.GetByIDFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, MaxVotesPerUser: 5}, nil
	}
	h.Repos.SessionRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (session.Session, error) {
		return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
	}
	h.Repos.TrackRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (track.RoomTrack, error) {
		return track.RoomTrack{ID: roomTrackID, RoomID: roomID, Status: track.StatusQueued, VoteCountCached: 2}, nil
	}
	h.Repos.VoteRepo.CountBySessionInRoomFn = func(context.Context, uuid.UUID, uuid.UUID) (int, error) {
		return 0, nil
	}
	h.Repos.VoteRepo.InsertFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
		return &pgconn.PgError{Code: "23505", ConstraintName: "uq_votes_session_track"}
	}

	_, err := h.Services.AddVote(context.Background(), roomID, session.Session{ID: sessionID}, roomTrackID)
	require.Error(t, err)

	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "ALREADY_VOTED_FOR_TRACK", apiErr.Code)
	require.Equal(t, http.StatusConflict, apiErr.Status)
	require.Empty(t, h.Broadcaster.Events)
}
