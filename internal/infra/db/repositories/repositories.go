package repositories

import (
	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/domain/room"
	"crowdbeats/internal/domain/session"
	"crowdbeats/internal/domain/spotify"
	"crowdbeats/internal/domain/track"
	"crowdbeats/internal/domain/vote"
	"crowdbeats/internal/usecase"
)

type Set struct {
	rooms    *RoomRepository
	sessions *SessionRepository
	spotify  *SpotifyRepository
	tracks   *TrackRepository
	queue    *QueueRepository
	votes    *VoteRepository
	events   *EventRepository
}

func NewSet(q Querier) *Set {
	return &Set{
		rooms:    &RoomRepository{q: q},
		sessions: &SessionRepository{q: q},
		spotify:  &SpotifyRepository{q: q},
		tracks:   &TrackRepository{q: q},
		queue:    &QueueRepository{q: q},
		votes:    &VoteRepository{q: q},
		events:   &EventRepository{q: q},
	}
}

func (s *Set) Rooms() room.Repository          { return s.rooms }
func (s *Set) Sessions() session.Repository    { return s.sessions }
func (s *Set) Spotify() spotify.Repository     { return s.spotify }
func (s *Set) Tracks() track.Repository        { return s.tracks }
func (s *Set) Queue() queue.Repository         { return s.queue }
func (s *Set) Votes() vote.Repository          { return s.votes }
func (s *Set) Events() usecase.EventRepository { return s.events }
