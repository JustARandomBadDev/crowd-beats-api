package repositories

import (
	"context"
	"database/sql"
	"time"

	"crowdbeats/internal/domain/queue"

	"github.com/google/uuid"
)

type QueueRepository struct{ q Querier }

func (r *QueueRepository) Load(ctx context.Context, roomID uuid.UUID) ([]queue.Item, *time.Time, error) {
	rows, err := r.q.Query(ctx, `
		select rq.position, rq.room_track_id, rq.score, rq.vote_count, rq.fifo_order, st.spotify_track_id, st.title, st.artist_names,
		       coalesce(st.album_name,''), coalesce(st.image_url,''), coalesce(st.preview_url,''), coalesce(st.uri,''),
		       us.nickname, rq.recalculated_at
		from room_queue rq
		join room_tracks rt on rt.id = rq.room_track_id
		join spotify_tracks st on st.id = rt.spotify_track_ref_id
		left join user_sessions us on us.id = rt.proposed_by_session_id
		where rq.room_id = $1
		order by rq.position asc
	`, roomID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]queue.Item, 0)
	var updatedAt *time.Time
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
			&item.ImageURL,
			&item.PreviewURL,
			&item.URI,
			&nickname,
			&item.RecalculatedAt,
		); err != nil {
			return nil, nil, err
		}
		if nickname.Valid {
			item.ProposedBy = &nickname.String
		}
		updatedAt = &item.RecalculatedAt
		items = append(items, item)
	}
	return items, updatedAt, rows.Err()
}

func (r *QueueRepository) ListRankedQueued(ctx context.Context, roomID uuid.UUID, limit int) ([]queue.RankedItem, error) {
	rows, err := r.q.Query(ctx, `
		select rt.id, count(v.id)::int as vote_count, rt.fifo_order
		from room_tracks rt
		left join votes v on v.room_track_id = rt.id
		where rt.room_id = $1 and rt.status = 'queued'
		group by rt.id, rt.fifo_order
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
		if err := rows.Scan(&item.RoomTrackID, &item.VoteCount, &item.FIFOOrder); err != nil {
			return nil, err
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
