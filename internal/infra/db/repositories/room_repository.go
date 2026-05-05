package repositories

import (
	"context"
	"time"

	"crowdbeats/internal/domain/room"

	"github.com/google/uuid"
)

type RoomRepository struct{ q Querier }

func (r *RoomRepository) Create(ctx context.Context, input room.CreateInput) (room.Room, error) {
	var out room.Room
	err := r.q.QueryRow(ctx, `
		insert into rooms (name, slug, status, queue_limit, max_votes_per_user, qr_ttl_seconds, recalc_interval_seconds, manager_secret_hash)
		values ($1, $2, $3, $4, $5, $6, $7, $8)
		returning id, name, slug, status, queue_limit, max_votes_per_user, qr_ttl_seconds, recalc_interval_seconds, manager_secret_hash, created_at, updated_at
	`, input.Name, input.Slug, input.Status, input.QueueLimit, input.MaxVotesPerUser, input.QRTTLSeconds, input.RecalcIntervalSeconds, input.ManagerSecretHash).Scan(
		&out.ID, &out.Name, &out.Slug, &out.Status, &out.QueueLimit, &out.MaxVotesPerUser, &out.QRTTLSeconds, &out.RecalcIntervalSeconds, &out.ManagerSecretHash, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func (r *RoomRepository) GetByID(ctx context.Context, roomID uuid.UUID) (room.Room, error) {
	var out room.Room
	err := r.q.QueryRow(ctx, `
		select id, name, slug, status, queue_limit, max_votes_per_user, qr_ttl_seconds, recalc_interval_seconds, manager_secret_hash, created_at, updated_at
		from rooms where id = $1
	`, roomID).Scan(
		&out.ID, &out.Name, &out.Slug, &out.Status, &out.QueueLimit, &out.MaxVotesPerUser, &out.QRTTLSeconds, &out.RecalcIntervalSeconds, &out.ManagerSecretHash, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func (r *RoomRepository) GetByQRCode(ctx context.Context, code string) (room.Room, error) {
	var out room.Room
	err := r.q.QueryRow(ctx, `
		select ro.id, ro.name, ro.slug, ro.status, ro.queue_limit, ro.max_votes_per_user, ro.qr_ttl_seconds, ro.recalc_interval_seconds, ro.manager_secret_hash, ro.created_at, ro.updated_at
		from room_qr_codes qr
		join rooms ro on ro.id = qr.room_id
		where qr.code = $1 and qr.revoked = false and qr.expires_at > now()
	`, code).Scan(
		&out.ID, &out.Name, &out.Slug, &out.Status, &out.QueueLimit, &out.MaxVotesPerUser, &out.QRTTLSeconds, &out.RecalcIntervalSeconds, &out.ManagerSecretHash, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func (r *RoomRepository) CreateQRCode(ctx context.Context, roomID uuid.UUID, code string, expiresAt time.Time) error {
	_, err := r.q.Exec(ctx, `insert into room_qr_codes (room_id, code, expires_at) values ($1, $2, $3)`, roomID, code, expiresAt)
	return err
}

func (r *RoomRepository) Patch(ctx context.Context, roomID uuid.UUID, patch room.Patch) (room.Room, error) {
	var out room.Room
	err := r.q.QueryRow(ctx, `
		update rooms
		set status = coalesce($2, status),
		    queue_limit = coalesce($3, queue_limit),
		    max_votes_per_user = coalesce($4, max_votes_per_user)
		where id = $1
		returning id, name, slug, status, queue_limit, max_votes_per_user, qr_ttl_seconds, recalc_interval_seconds, manager_secret_hash, created_at, updated_at
	`, roomID, patch.Status, patch.QueueLimit, patch.MaxVotesPerUser).Scan(
		&out.ID, &out.Name, &out.Slug, &out.Status, &out.QueueLimit, &out.MaxVotesPerUser, &out.QRTTLSeconds, &out.RecalcIntervalSeconds, &out.ManagerSecretHash, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

func (r *RoomRepository) LoadStats(ctx context.Context, roomID uuid.UUID) (room.Stats, error) {
	var stats room.Stats
	if err := r.q.QueryRow(ctx, `
		select
			(select count(*) from user_sessions where room_id = $1 and status = 'active'),
			(select count(*) from room_tracks where room_id = $1 and status in ('queued','playing')),
			(select count(*) from votes where room_id = $1)
	`, roomID).Scan(&stats.ActiveUsers, &stats.TracksInQueue, &stats.VotesCount); err != nil {
		return stats, err
	}
	rows, err := r.q.Query(ctx, `
		select st.title, st.artist_names, count(v.id) as votes
		from room_tracks rt
		join spotify_tracks st on st.id = rt.spotify_track_ref_id
		left join votes v on v.room_track_id = rt.id
		where rt.room_id = $1 and rt.status in ('queued','playing')
		group by st.title, st.artist_names, rt.fifo_order
		order by votes desc, rt.fifo_order asc
		limit 5
	`, roomID)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var top room.TopTrack
		if err := rows.Scan(&top.Title, &top.ArtistNames, &top.Votes); err != nil {
			return stats, err
		}
		stats.TopTracks = append(stats.TopTracks, top)
	}
	return stats, rows.Err()
}

func (r *RoomRepository) CountActiveUsers(ctx context.Context, roomID uuid.UUID) (int, error) {
	var count int
	err := r.q.QueryRow(ctx, `select count(*) from user_sessions where room_id = $1 and status = 'active'`, roomID).Scan(&count)
	return count, err
}
