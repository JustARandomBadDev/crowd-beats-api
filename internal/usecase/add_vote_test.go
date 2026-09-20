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
		return room.Room{ID: roomID, Status: room.StatusActive, MaxVotesPerUser: 5}, nil
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

func TestAddVoteRequiresActiveRoom(t *testing.T) {
	for _, status := range []string{room.StatusDraft, room.StatusPaused, room.StatusClosed} {
		t.Run(status, func(t *testing.T) {
			h := testutil.NewHarness()
			roomID := uuid.New()
			h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
				return room.Room{ID: roomID, Status: status, MaxVotesPerUser: 5}, nil
			}
			h.Repos.VoteRepo.InsertFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
				t.Fatal("vote must not be inserted")
				return nil
			}
			_, err := h.Services.AddVote(context.Background(), roomID, session.Session{ID: uuid.New()}, uuid.New())
			var apiErr apierror.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, "ROOM_NOT_ACTIVE", apiErr.Code)
			require.Equal(t, http.StatusForbidden, apiErr.Status)
		})
	}
}

func TestAddVoteActiveRoomAndQuota(t *testing.T) {
	for _, used := range []int{4, 5} {
		h := testutil.NewHarness()
		roomID, sessionID, trackID := uuid.New(), uuid.New(), uuid.New()
		h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
			return room.Room{ID: roomID, Status: room.StatusActive, MaxVotesPerUser: 5}, nil
		}
		h.Repos.SessionRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (session.Session, error) {
			return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
		}
		h.Repos.TrackRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (track.RoomTrack, error) {
			return track.RoomTrack{ID: trackID, RoomID: roomID, Status: track.StatusQueued}, nil
		}
		h.Repos.VoteRepo.CountBySessionInRoomFn = func(context.Context, uuid.UUID, uuid.UUID) (int, error) {
			return used, nil
		}
		inserted := false
		h.Repos.VoteRepo.InsertFn = func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
			inserted = true
			return nil
		}
		result, err := h.Services.AddVote(context.Background(), roomID, session.Session{ID: sessionID}, trackID)
		if used == 4 {
			require.NoError(t, err)
			require.True(t, inserted)
			require.True(t, result.VoteAdded)
			require.Equal(t, 0, result.VotesRemaining)
		} else {
			var apiErr apierror.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, "VOTE_LIMIT_REACHED", apiErr.Code)
			require.False(t, inserted)
		}
	}
}

func TestAddVoteDuplicateTakesPriorityOverExhaustedQuota(t *testing.T) {
	h := testutil.NewHarness()
	roomID, sessionID, trackID := uuid.New(), uuid.New(), uuid.New()
	h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
		return room.Room{ID: roomID, Status: room.StatusActive, MaxVotesPerUser: 1}, nil
	}
	h.Repos.SessionRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (session.Session, error) {
		return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
	}
	h.Repos.TrackRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (track.RoomTrack, error) {
		return track.RoomTrack{ID: trackID, RoomID: roomID, Status: track.StatusQueued}, nil
	}
	h.Repos.VoteRepo.HasBySessionAndTrackFn = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
		return true, nil
	}
	h.Repos.VoteRepo.CountBySessionInRoomFn = func(context.Context, uuid.UUID, uuid.UUID) (int, error) {
		t.Fatal("quota must be checked after duplicate")
		return 0, nil
	}
	_, err := h.Services.AddVote(context.Background(), roomID, session.Session{ID: sessionID}, trackID)
	var apiErr apierror.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "ALREADY_VOTED_FOR_TRACK", apiErr.Code)
	require.Equal(t, http.StatusConflict, apiErr.Status)
}

func TestAddVoteRejectsWrongRoomAndTrackState(t *testing.T) {
	for _, tc := range []struct {
		name      string
		trackRoom uuid.UUID
		status    string
	}{
		{name: "wrong room", trackRoom: uuid.New(), status: track.StatusQueued},
		{name: "played", status: track.StatusPlayed},
		{name: "skipped", status: track.StatusSkipped},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := testutil.NewHarness()
			roomID, sessionID := uuid.New(), uuid.New()
			h.Repos.RoomRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (room.Room, error) {
				return room.Room{ID: roomID, Status: room.StatusActive, MaxVotesPerUser: 5}, nil
			}
			h.Repos.SessionRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (session.Session, error) {
				return session.Session{ID: sessionID, RoomID: roomID, Status: session.StatusActive}, nil
			}
			if tc.trackRoom == uuid.Nil {
				tc.trackRoom = roomID
			}
			h.Repos.TrackRepo.GetByIDForUpdateFn = func(context.Context, uuid.UUID) (track.RoomTrack, error) {
				return track.RoomTrack{RoomID: tc.trackRoom, Status: tc.status}, nil
			}
			_, err := h.Services.AddVote(context.Background(), roomID, session.Session{ID: sessionID}, uuid.New())
			var apiErr apierror.Error
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, "ROOM_TRACK_NOT_ACTIVE", apiErr.Code)
		})
	}
}
