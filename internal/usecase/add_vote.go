package usecase

import (
	"context"
	"net/http"

	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/track"
	"crowdbeats/pkg/apierror"

	"github.com/google/uuid"
)

func (s *Services) AddVote(ctx context.Context, roomID uuid.UUID, current session.Session, roomTrackID uuid.UUID) (map[string]any, error) {
	var response map[string]any
	err := s.UOW.Run(ctx, func(repos RepositorySet) error {
		targetRoom, err := repos.Rooms().GetByID(ctx, roomID)
		if err != nil {
			return err
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
			return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track not found", http.StatusNotFound)
		}
		if targetTrack.RoomID != roomID || (targetTrack.Status != track.StatusQueued && targetTrack.Status != track.StatusPlaying) {
			return apierror.New("ROOM_TRACK_NOT_ACTIVE", "track is not active", http.StatusConflict)
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
		response = map[string]any{
			"vote_added":         true,
			"room_track_id":      roomTrackID,
			"current_vote_count": currentVoteCount,
			"votes_remaining":    targetRoom.MaxVotesPerUser - usedVotes - 1,
		}
		s.DirtyRooms.Mark(roomID)
		s.Broadcaster.Broadcast(LiveEvent{
			Name:      "vote_received",
			RoomID:    roomID,
			Timestamp: s.Now(),
			Payload: map[string]any{
				"room_track_id":      roomTrackID,
				"current_vote_count": currentVoteCount,
				"votes_remaining":    targetRoom.MaxVotesPerUser - usedVotes - 1,
			},
		})
		return nil
	})
	return response, err
}
