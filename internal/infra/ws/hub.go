package ws

import (
	"encoding/json"
	"sync"
	"time"

	"crowdbeats/internal/domain/queue"
	"crowdbeats/internal/infra/http/dto"
	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
)

// RoomHub keeps pending clients out of broadcasts until their post-register
// session check succeeds. Its mutex also serializes sends with closing send.
type RoomHub struct {
	mu      sync.Mutex
	clients map[*Client]bool
}

func NewRoomHub() *RoomHub {
	return &RoomHub{clients: make(map[*Client]bool)}
}

func (h *RoomHub) registerPending(client *Client) {
	h.mu.Lock()
	h.clients[client] = false
	h.mu.Unlock()
}

func (h *RoomHub) activate(client *Client, syncMessage []byte) bool {
	h.mu.Lock()
	_, registered := h.clients[client]
	if registered {
		select {
		case client.send <- syncMessage:
			h.clients[client] = true
		default:
			delete(h.clients, client)
			registered = false
		}
	}
	h.mu.Unlock()
	if !registered {
		client.Close()
	}
	return registered
}

func (h *RoomHub) unregister(client *Client) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
	client.Close()
}

func (h *RoomHub) disconnectSession(sessionID uuid.UUID) {
	h.mu.Lock()
	closing := make([]*Client, 0)
	for client := range h.clients {
		if client.sessionID == sessionID {
			delete(h.clients, client)
			closing = append(closing, client)
		}
	}
	h.mu.Unlock()
	for _, client := range closing {
		client.Close()
	}
}

func (h *RoomHub) broadcast(message []byte) {
	h.mu.Lock()
	closing := make([]*Client, 0)
	for client, active := range h.clients {
		if !active {
			continue
		}
		select {
		case client.send <- message:
		default:
			delete(h.clients, client)
			closing = append(closing, client)
		}
	}
	h.mu.Unlock()
	for _, client := range closing {
		client.Close()
	}
}

func (h *RoomHub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

type Registry struct {
	mu   sync.RWMutex
	hubs map[uuid.UUID]*RoomHub
}

func NewRegistry() *Registry {
	return &Registry{hubs: make(map[uuid.UUID]*RoomHub)}
}

func (r *Registry) hub(roomID uuid.UUID) *RoomHub {
	r.mu.Lock()
	defer r.mu.Unlock()
	if hub, ok := r.hubs[roomID]; ok {
		return hub
	}
	hub := NewRoomHub()
	r.hubs[roomID] = hub
	return hub
}

func (r *Registry) existingHub(roomID uuid.UUID) *RoomHub {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hubs[roomID]
}

func (r *Registry) RegisterPending(roomID uuid.UUID, client *Client) {
	r.hub(roomID).registerPending(client)
}

func (r *Registry) Activate(roomID uuid.UUID, client *Client) bool {
	payload, err := json.Marshal(Event{
		Event: "sync_required", RoomID: roomID, Timestamp: time.Now().UTC(),
		Payload: map[string]any{},
	})
	if err != nil {
		return false
	}
	return r.hub(roomID).activate(client, payload)
}

func (r *Registry) Unregister(roomID uuid.UUID, client *Client) {
	if hub := r.existingHub(roomID); hub != nil {
		hub.unregister(client)
	} else {
		client.Close()
	}
}

func (r *Registry) DisconnectSession(roomID, sessionID uuid.UUID) {
	if hub := r.existingHub(roomID); hub != nil {
		hub.disconnectSession(sessionID)
	}
}

func (r *Registry) Broadcast(event usecase.LiveEvent) {
	hub := r.existingHub(event.RoomID)
	if hub == nil {
		return
	}
	payloadData := event.Payload
	if event.Name == "queue_updated" {
		snapshot, ok := event.Payload.(queue.Snapshot)
		if !ok {
			return
		}
		payloadData = dto.QueueSnapshotFromDomain(snapshot)
	}
	payload, err := json.Marshal(Event{
		Event: event.Name, RoomID: event.RoomID,
		Timestamp: event.Timestamp, Payload: payloadData,
	})
	if err != nil {
		return
	}
	hub.broadcast(payload)
}
