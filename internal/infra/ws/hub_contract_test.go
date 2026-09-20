package ws

import (
	"strings"
	"testing"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestQueueUpdatedUsesPublicQueueItems(t *testing.T) {
	registry := NewRegistry()
	roomID := uuid.New()
	client := &Client{send: make(chan []byte, 1)}
	hub := registry.hub(roomID)
	hub.register <- client
	defer func() { hub.unregister <- client }()

	updatedAt := time.Now().UTC()
	registry.Broadcast(usecase.LiveEvent{
		Name: "queue_updated", RoomID: roomID, Timestamp: updatedAt,
		Payload: map[string]any{
			"updated_at": &updatedAt,
			"items":      []queue.Item{{RoomTrackID: uuid.New(), SpotifyTrackID: "spotify-id", Title: "Song", FIFOOrder: 99, RecalculatedAt: updatedAt}},
		},
	})

	select {
	case message := <-client.send:
		body := string(message)
		require.Contains(t, body, `"event":"queue_updated"`)
		require.Contains(t, body, `"spotify_track_id":"spotify-id"`)
		require.NotContains(t, strings.ToLower(body), "fifoorder")
		require.NotContains(t, strings.ToLower(body), "fifo_order")
		require.NotContains(t, strings.ToLower(body), "recalculatedat")
		require.NotContains(t, strings.ToLower(body), "recalculated_at")
	case <-time.After(time.Second):
		t.Fatal("queue_updated was not broadcast")
	}
}
