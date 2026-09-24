package ws

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func registeredClient(t *testing.T, registry *Registry, roomID, sessionID uuid.UUID, buffer int) *Client {
	t.Helper()
	client := newClient(nil, roomID, sessionID, buffer)
	registry.RegisterPending(roomID, client)
	require.True(t, registry.Activate(roomID, client))
	select {
	case message := <-client.send:
		require.Contains(t, string(message), `"event":"sync_required"`)
	case <-time.After(time.Second):
		t.Fatal("sync_required not delivered")
	}
	return client
}

func TestQueueUpdatedUsesSamePublicSnapshotAsREST(t *testing.T) {
	registry := NewRegistry()
	roomID := uuid.New()
	client := registeredClient(t, registry, roomID, uuid.New(), 1)
	defer registry.Unregister(roomID, client)
	updatedAt := time.Now().UTC()
	snapshot := queue.Snapshot{
		Items:      []queue.Item{{RoomTrackID: uuid.New(), Position: 1, SpotifyTrackID: "spotify-id", Title: "Queued", FIFOOrder: 99}},
		NowPlaying: &queue.Item{RoomTrackID: uuid.New(), SpotifyTrackID: "playing-id", Title: "Playing", VoteCount: 3},
		UpdatedAt:  &updatedAt,
	}
	registry.Broadcast(usecase.LiveEvent{
		Name: "queue_updated", RoomID: roomID, Timestamp: updatedAt, Payload: snapshot,
	})
	select {
	case message := <-client.send:
		var event struct {
			Event   string          `json:"event"`
			Payload json.RawMessage `json:"payload"`
		}
		require.NoError(t, json.Unmarshal(message, &event))
		require.Equal(t, "queue_updated", event.Event)
		restPayload, err := json.Marshal(dto.QueueSnapshotFromDomain(snapshot))
		require.NoError(t, err)
		require.JSONEq(t, string(restPayload), string(event.Payload))
		require.NotContains(t, strings.ToLower(string(message)), "fifo_order")
		require.NotContains(t, string(event.Payload), `"position":0`)
	case <-time.After(time.Second):
		t.Fatal("queue_updated was not broadcast")
	}
}

func TestDisconnectSessionClosesAllMatchingClientsOnly(t *testing.T) {
	registry := NewRegistry()
	roomID, sessionA, sessionB := uuid.New(), uuid.New(), uuid.New()
	a1 := registeredClient(t, registry, roomID, sessionA, 2)
	a2 := registeredClient(t, registry, roomID, sessionA, 2)
	b := registeredClient(t, registry, roomID, sessionB, 2)
	registry.DisconnectSession(roomID, sessionA)
	require.Equal(t, 1, registry.hub(roomID).clientCount())
	for _, client := range []*Client{a1, a2} {
		select {
		case <-client.closed:
		default:
			t.Fatal("session A client not closed")
		}
	}
	select {
	case <-b.closed:
		t.Fatal("session B client was closed")
	default:
	}
	registry.Broadcast(usecase.LiveEvent{Name: "presence_updated", RoomID: roomID, Timestamp: time.Now(), Payload: map[string]any{"active_users": 1}})
	require.Contains(t, string(<-b.send), `"presence_updated"`)
	registry.Unregister(roomID, b)
}

func TestSlowClientIsClosedAndRemoved(t *testing.T) {
	registry := NewRegistry()
	roomID := uuid.New()
	client := registeredClient(t, registry, roomID, uuid.New(), 1)
	client.send <- []byte("buffer already full")
	registry.Broadcast(usecase.LiveEvent{Name: "presence_updated", RoomID: roomID, Timestamp: time.Now(), Payload: map[string]any{}})
	require.Zero(t, registry.hub(roomID).clientCount())
	select {
	case <-client.closed:
	default:
		t.Fatal("slow client connection was not closed")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	client := newClient(nil, uuid.New(), uuid.New(), 1)
	client.Close()
	client.Close()
	select {
	case <-client.closed:
	default:
		t.Fatal("client still open")
	}
}
