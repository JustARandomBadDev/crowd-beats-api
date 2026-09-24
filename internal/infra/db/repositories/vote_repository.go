package repositories

import (
	"context"

	"github.com/google/uuid"
)

type VoteRepository struct{ q Querier }

func (r *VoteRepository) CountBySessionInRoom(ctx context.Context, roomID, sessionID uuid.UUID) (int, error) {
	var count int
	err := r.q.QueryRow(ctx, `select count(*) from votes where room_id = $1 and session_id = $2`, roomID, sessionID).Scan(&count)
	return count, err
}

func (r *VoteRepository) HasBySessionAndTrack(ctx context.Context, sessionID, roomTrackID uuid.UUID) (bool, error) {
	var exists bool
	err := r.q.QueryRow(ctx, `select exists(select 1 from votes where session_id = $1 and room_track_id = $2)`, sessionID, roomTrackID).Scan(&exists)
	return exists, err
}

func (r *VoteRepository) Insert(ctx context.Context, roomID, roomTrackID, sessionID uuid.UUID) error {
	_, err := r.q.Exec(ctx, `insert into votes (room_id, room_track_id, session_id) values ($1, $2, $3)`, roomID, roomTrackID, sessionID)
	return err
}
