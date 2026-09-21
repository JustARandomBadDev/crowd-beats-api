package usecase

import (
	"context"
	"errors"
	"net/http"

	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/track"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AddVoteResult struct {
	VoteAdded        bool
	RoomTrackID      uuid.UUID
	CurrentVoteCount int
	VotesRemaining   int
}

func (s *Services) AddVote(ctx context.Context, roomID uuid.UUID, current session.Session, roomTrackID uuid.UUID) (AddVoteResult, error) {
	var response AddVoteResult
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByIDForUpdate(ctx, roomID)
		if err != nil {
			return err
		}
		if targetRoom.Status != room.StatusActive {
			return apierror.New("ROOM_NOT_ACTIVE", "room is not active", http.StatusForbidden)
		}
		lockedSession, err := repos.Sessions().GetByIDForUpdate(ctx, current.ID)
		if err != nil {
			return err
		}
		if lockedSession.RoomID != roomID || lockedSession.Status != session.StatusActive {
			return apierror.New("SESSION_NOT_IN_ROOM", "session is not active in room", http.StatusForbidden)
		}

		targetTrack, err := repos.Tracks().GetByIDForUpdate(ctx, roomTrackID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track not found", http.StatusNotFound)
			}
			return err
		}
		if targetTrack.RoomID != roomID || (targetTrack.Status != track.StatusQueued && targetTrack.Status != track.StatusPlaying) {
			return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track is not active", http.StatusConflict)
		}
		alreadyVoted, err := repos.Votes().HasBySessionAndTrack(ctx, current.ID, roomTrackID)
		if err != nil {
			return err
		}
		if alreadyVoted {
			return apierror.New("ALREADY_VOTED_FOR_TRACK", "track already voted by this session", http.StatusConflict)
		}

		usedVotes, err := repos.Votes().CountBySessionInRoom(ctx, roomID, current.ID)
		if err != nil {
			return err
		}
		if usedVotes >= targetRoom.MaxVotesPerUser {
			return apierror.New("VOTE_LIMIT_REACHED", "vote limit reached", http.StatusConflict)
		}
		if err := repos.Votes().Insert(ctx, roomID, roomTrackID, current.ID); err != nil {
			if isUniqueConstraint(err, "uq_votes_session_track") {
				return apierror.New("ALREADY_VOTED_FOR_TRACK", "track already voted by this session", http.StatusConflict)
			}
			return err
		}
		if err := repos.Events().Insert(ctx, roomID, "vote_added", map[string]any{
			"room_track_id": roomTrackID,
			"session_id":    current.ID,
		}); err != nil {
			return err
		}

		currentVoteCount := targetTrack.VoteCountCached + 1
		response = AddVoteResult{
			VoteAdded: true, RoomTrackID: roomTrackID,
			CurrentVoteCount: currentVoteCount,
			VotesRemaining:   targetRoom.MaxVotesPerUser - usedVotes - 1,
		}
		return nil
	})
	if err == nil {
		s.Broadcaster.Broadcast(LiveEvent{
			Name: "vote_received", RoomID: roomID, Timestamp: s.Now(),
			Payload: map[string]any{
				"room_track_id": roomTrackID, "current_vote_count": response.CurrentVoteCount,
			},
		})
	}
	return response, err
}
