package repositories

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"crowdbeats/internal/domain/queue"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type QueueRepository struct{ q Querier }

func (r *QueueRepository) LoadSnapshot(ctx context.Context, roomID uuid.UUID) (queue.Snapshot, error) {
	state, err := r.LoadState(ctx, roomID)
	if err != nil {
		return queue.Snapshot{}, err
	}
	playing, err := r.LoadNowPlaying(ctx, roomID)
	if err != nil {
		return queue.Snapshot{}, err
	}
	rows, err := r.q.Query(ctx, `
		select rq.position, rq.room_track_id, rq.score, rq.vote_count, rq.fifo_order, st.spotify_track_id, st.title, st.artist_names,
		       coalesce(st.album_name,''), coalesce(st.duration_ms,0), coalesce(st.image_url,''), coalesce(st.preview_url,''), coalesce(st.uri,''),
		       us.nickname, rq.recalculated_at
		from room_queue rq
		join room_tracks rt on rt.id = rq.room_track_id
		join spotify_tracks st on st.id = rt.spotify_track_ref_id
		left join user_sessions us on us.id = rt.proposed_by_session_id
		where rq.room_id = $1 and rt.status = 'queued'
		order by rq.position asc
	`, roomID)
	if err != nil {
		return queue.Snapshot{}, err
	}
	defer rows.Close()

	snapshot := queue.Snapshot{Items: make([]queue.Item, 0), NowPlaying: playing, UpdatedAt: state.UpdatedAt}
	for rows.Next() {
		var item queue.Item
		var nickname sql.NullString
		if err := rows.Scan(
			&item.Position,
			&item.RoomTrackID,
			&item.Score,
			&item.VoteCount,
			&item.FIFOOrder,
			&item.SpotifyTrackID,
			&item.Title,
			&item.ArtistNames,
			&item.AlbumName,
			&item.DurationMS,
			&item.ImageURL,
			&item.PreviewURL,
			&item.URI,
			&nickname,
			&item.RecalculatedAt,
		); err != nil {
			return queue.Snapshot{}, err
		}
		if nickname.Valid {
			item.ProposedBy = &nickname.String
		}
		// A deleted or skipped row may leave a temporary position gap until the
		// next batch. Preserve stored ranking while exposing contiguous positions.
		item.Position = len(snapshot.Items) + 1
		snapshot.Items = append(snapshot.Items, item)
	}
	return snapshot, rows.Err()
}

func (r *QueueRepository) LoadState(ctx context.Context, roomID uuid.UUID) (queue.SnapshotState, error) {
	var state queue.SnapshotState
	var updatedAt time.Time
	err := r.q.QueryRow(ctx, `
		select coalesce(payload->>'fingerprint', ''),
		       coalesce((payload->>'updated_at')::timestamptz, created_at)
		from room_events
		where room_id = $1 and event_type = 'queue_recalculated'
		-- created_at defaults to transaction start, which can precede another
		-- recalculation while waiting for the room lock. The payload timestamp
		-- is assigned after that lock has been acquired.
		order by coalesce((payload->>'updated_at')::timestamptz, created_at) desc, created_at desc, id desc
		limit 1
	`, roomID).Scan(&state.Fingerprint, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return queue.SnapshotState{}, nil
	}
	if err != nil {
		return queue.SnapshotState{}, err
	}
	state.UpdatedAt = &updatedAt
	return state, nil
}

func (r *QueueRepository) LoadNowPlaying(ctx context.Context, roomID uuid.UUID) (*queue.Item, error) {
	var item queue.Item
	var nickname sql.NullString
	err := r.q.QueryRow(ctx, `
		select rt.id, rt.vote_count_cached, st.spotify_track_id, st.title, st.artist_names,
		       coalesce(st.album_name,''), coalesce(st.duration_ms,0), coalesce(st.image_url,''),
		       coalesce(st.preview_url,''), coalesce(st.uri,''), us.nickname
		from room_tracks rt
		join spotify_tracks st on st.id = rt.spotify_track_ref_id
		left join user_sessions us on us.id = rt.proposed_by_session_id
		where rt.room_id = $1 and rt.status = 'playing'
		order by rt.updated_at desc, rt.fifo_order asc
		limit 1
	`, roomID).Scan(
		&item.RoomTrackID, &item.VoteCount, &item.SpotifyTrackID, &item.Title,
		&item.ArtistNames, &item.AlbumName, &item.DurationMS, &item.ImageURL,
		&item.PreviewURL, &item.URI, &nickname,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if nickname.Valid {
		item.ProposedBy = &nickname.String
	}
	return &item, nil
}

func (r *QueueRepository) ListRankedQueued(ctx context.Context, roomID uuid.UUID, limit int) ([]queue.RankedItem, error) {
	rows, err := r.q.Query(ctx, `
		select rt.id, count(v.id)::int as vote_count, rt.fifo_order, us.nickname
		from room_tracks rt
		left join votes v on v.room_track_id = rt.id
		left join user_sessions us on us.id = rt.proposed_by_session_id
		where rt.room_id = $1 and rt.status = 'queued'
		group by rt.id, rt.fifo_order, us.nickname
		order by count(v.id) desc, rt.fifo_order asc
		limit $2
	`, roomID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []queue.RankedItem
	for rows.Next() {
		var item queue.RankedItem
		var nickname sql.NullString
		if err := rows.Scan(&item.RoomTrackID, &item.VoteCount, &item.FIFOOrder, &nickname); err != nil {
			return nil, err
		}
		if nickname.Valid {
			item.ProposedBy = &nickname.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *QueueRepository) Replace(ctx context.Context, roomID uuid.UUID, items []queue.RankedItem, recalculatedAt time.Time) error {
	if _, err := r.q.Exec(ctx, `delete from room_queue where room_id = $1`, roomID); err != nil {
		return err
	}
	for idx, item := range items {
		if _, err := r.q.Exec(ctx, `
			insert into room_queue (room_id, room_track_id, position, score, vote_count, fifo_order, recalculated_at)
			values ($1, $2, $3, $4, $5, $6, $7)
		`, roomID, item.RoomTrackID, idx+1, item.VoteCount, item.VoteCount, item.FIFOOrder, recalculatedAt); err != nil {
			return err
		}
	}
	return nil
}
