package repositories

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

type EventRepository struct{ q Querier }

func (r *EventRepository) Insert(ctx context.Context, roomID uuid.UUID, eventType string, payload map[string]any) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = r.q.Exec(ctx, `insert into room_events (room_id, event_type, payload) values ($1, $2, $3)`, roomID, eventType, bytes)
	return err
}
