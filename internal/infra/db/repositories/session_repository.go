package repositories

import (
	"context"

	"crowdbeats/internal/domain/session"

	"github.com/google/uuid"
)

type SessionRepository struct{ q Querier }

func (r *SessionRepository) GetByTokenHash(ctx context.Context, hash string) (session.Session, error) {
	var out session.Session
	err := r.q.QueryRow(ctx, `
		select id, room_id, nickname, role, status, session_token_hash, last_seen_at
		from user_sessions
		where session_token_hash = $1
	`, hash).Scan(&out.ID, &out.RoomID, &out.Nickname, &out.Role, &out.Status, &out.SessionTokenHash, &out.LastSeenAt)
	return out, err
}

func (r *SessionRepository) Create(ctx context.Context, input session.CreateInput) (session.Session, error) {
	var out session.Session
	err := r.q.QueryRow(ctx, `
		insert into user_sessions (room_id, session_token_hash, nickname, role)
		values ($1, $2, $3, $4)
		returning id, room_id, nickname, role, status, session_token_hash, last_seen_at
	`, input.RoomID, input.SessionTokenHash, input.Nickname, input.Role).Scan(
		&out.ID, &out.RoomID, &out.Nickname, &out.Role, &out.Status, &out.SessionTokenHash, &out.LastSeenAt,
	)
	return out, err
}

func (r *SessionRepository) Reattach(ctx context.Context, sessionID uuid.UUID, roomID uuid.UUID, nickname string) error {
	_, err := r.q.Exec(ctx, `
		update user_sessions
		set room_id = $2,
		    nickname = $3,
		    role = 'guest',
		    status = 'active',
		    joined_at = now(),
		    last_seen_at = now(),
		    left_at = null
		where id = $1
	`, sessionID, roomID, nickname)
	return err
}

func (r *SessionRepository) GetByIDForUpdate(ctx context.Context, sessionID uuid.UUID) (session.Session, error) {
	var out session.Session
	err := r.q.QueryRow(ctx, `
		select id, room_id, nickname, role, status, session_token_hash, last_seen_at
		from user_sessions
		where id = $1
		for update
	`, sessionID).Scan(&out.ID, &out.RoomID, &out.Nickname, &out.Role, &out.Status, &out.SessionTokenHash, &out.LastSeenAt)
	return out, err
}

func (r *SessionRepository) Heartbeat(ctx context.Context, sessionID uuid.UUID) error {
	_, err := r.q.Exec(ctx, `update user_sessions set last_seen_at = now(), status = 'active' where id = $1`, sessionID)
	return err
}

func (r *SessionRepository) Leave(ctx context.Context, sessionID uuid.UUID) error {
	_, err := r.q.Exec(ctx, `update user_sessions set status = 'left', left_at = now(), last_seen_at = now() where id = $1`, sessionID)
	return err
}
