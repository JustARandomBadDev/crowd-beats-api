package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"crowdbeats/internal/domain/track"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type TrackRepository struct{ q Querier }

func (r *TrackRepository) CountActive(ctx context.Context, roomID uuid.UUID) (int, error) {
	var count int
	err := r.q.QueryRow(ctx, `select count(*) from room_tracks where room_id = $1 and status in ('queued','playing')`, roomID).Scan(&count)
	return count, err
}

func (r *TrackRepository) CreateQueued(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID, sessionID uuid.UUID) (track.RoomTrack, error) {
	var out track.RoomTrack
	err := r.q.QueryRow(ctx, `
		insert into room_tracks (room_id, spotify_track_ref_id, proposed_by_session_id)
		values ($1, $2, $3)
		returning id, room_id, spotify_track_ref_id, proposed_by_session_id, status, vote_count_cached, score_cached, fifo_order, proposed_at
	`, roomID, spotifyTrackRefID, sessionID).Scan(
		&out.ID, &out.RoomID, &out.SpotifyTrackRefID, &out.ProposedBySessionID, &out.Status, &out.VoteCountCached, &out.ScoreCached, &out.FIFOOrder, &out.ProposedAt,
	)
	return out, err
}

func (r *TrackRepository) CreateQueuedIfAbsent(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID, sessionID uuid.UUID) (track.RoomTrack, bool, error) {
	var out track.RoomTrack
	err := r.q.QueryRow(ctx, `
		insert into room_tracks (room_id, spotify_track_ref_id, proposed_by_session_id)
		values ($1, $2, $3)
		on conflict (room_id, spotify_track_ref_id) where status in ('queued', 'playing') do nothing
		returning id, room_id, spotify_track_ref_id, proposed_by_session_id, status, vote_count_cached, score_cached, fifo_order, proposed_at
	`, roomID, spotifyTrackRefID, sessionID).Scan(
		&out.ID, &out.RoomID, &out.SpotifyTrackRefID, &out.ProposedBySessionID, &out.Status, &out.VoteCountCached, &out.ScoreCached, &out.FIFOOrder, &out.ProposedAt,
	)
	if err == pgx.ErrNoRows {
		return track.RoomTrack{}, false, nil
	}
	return out, err == nil, err
}

func (r *TrackRepository) GetActiveDuplicate(ctx context.Context, roomID uuid.UUID, spotifyTrackRefID uuid.UUID) (track.DuplicateInfo, error) {
	var out track.DuplicateInfo
	var position sql.NullInt32
	err := r.q.QueryRow(ctx, `
		select rt.id, rt.vote_count_cached, rq.position
		from room_tracks rt
		left join room_queue rq on rq.room_track_id = rt.id and rq.room_id = rt.room_id
		where rt.room_id = $1 and rt.spotify_track_ref_id = $2 and rt.status in ('queued','playing')
		limit 1
	`, roomID, spotifyTrackRefID).Scan(&out.ID, &out.CurrentVoteCount, &position)
	out.Position = nullablePosition(position)
	return out, err
}

func (r *TrackRepository) GetByIDForUpdate(ctx context.Context, roomTrackID uuid.UUID) (track.RoomTrack, error) {
	var out track.RoomTrack
	err := r.q.QueryRow(ctx, `
		select id, room_id, spotify_track_ref_id, proposed_by_session_id, status, vote_count_cached, score_cached, fifo_order, proposed_at
		from room_tracks
		where id = $1
		for update
	`, roomTrackID).Scan(
		&out.ID, &out.RoomID, &out.SpotifyTrackRefID, &out.ProposedBySessionID, &out.Status, &out.VoteCountCached, &out.ScoreCached, &out.FIFOOrder, &out.ProposedAt,
	)
	return out, err
}

func (r *TrackRepository) Delete(ctx context.Context, roomID, roomTrackID uuid.UUID) (bool, error) {
	tag, err := r.q.Exec(ctx, `delete from room_tracks where room_id = $1 and id = $2`, roomID, roomTrackID)
	return tag.RowsAffected() > 0, err
}

func (r *TrackRepository) SetStatus(ctx context.Context, roomID, roomTrackID uuid.UUID, status string, timestampColumn string) (bool, error) {
	if !allowedStatusColumn(timestampColumn) {
		return false, fmt.Errorf("invalid timestamp column")
	}
	query := fmt.Sprintf(`update room_tracks set status = $3, %s = now() where room_id = $1 and id = $2 and status in ('queued','playing')`, timestampColumn)
	tag, err := r.q.Exec(ctx, query, roomID, roomTrackID, status)
	return tag.RowsAffected() > 0, err
}

func (r *TrackRepository) SetPlaying(ctx context.Context, roomID, roomTrackID uuid.UUID) (bool, error) {
	if _, err := r.q.Exec(ctx, `update room_tracks set status = 'queued', played_at = null where room_id = $1 and status = 'playing'`, roomID); err != nil {
		return false, err
	}
	tag, err := r.q.Exec(ctx, `update room_tracks set status = 'playing' where room_id = $1 and id = $2 and status in ('queued','playing')`, roomID, roomTrackID)
	return tag.RowsAffected() > 0, err
}

func (r *TrackRepository) UpdateCachedScores(ctx context.Context, roomID uuid.UUID, ranked map[uuid.UUID]int) error {
	ids := make([]uuid.UUID, 0, len(ranked))
	for roomTrackID, score := range ranked {
		ids = append(ids, roomTrackID)
		if _, err := r.q.Exec(ctx, `update room_tracks set vote_count_cached = $2, score_cached = $2 where id = $1`, roomTrackID, score); err != nil {
			return err
		}
	}
	_, err := r.q.Exec(ctx, `
		update room_tracks
		set score_cached = 0
		where room_id = $1 and status = 'queued' and id <> all(coalesce($2::uuid[], array[]::uuid[]))
	`, roomID, ids)
	return err
}

func allowedStatusColumn(column string) bool {
	switch strings.TrimSpace(column) {
	case "skipped_at", "played_at", "deleted_at":
		return true
	default:
		return false
	}
}

func nullablePosition(v sql.NullInt32) *int {
	if !v.Valid {
		return nil
	}
	value := int(v.Int32)
	return &value
}
