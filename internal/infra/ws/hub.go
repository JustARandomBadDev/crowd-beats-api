package ws

import (
	"encoding/json"
	"sync"

	"crowdbeats/internal/usecase"

	"github.com/google/uuid"
)

type RoomHub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	clients    map[*Client]struct{}
}

func NewRoomHub() *RoomHub {
	h := &RoomHub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 32),
		clients:    map[*Client]struct{}{},
	}
	go h.run()
	return h
}

func (h *RoomHub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					delete(h.clients, client)
					close(client.send)
				}
			}
		}
	}
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

func (r *Registry) Broadcast(event usecase.LiveEvent) {
	payload, err := json.Marshal(Event{
		Event:     event.Name,
		RoomID:    event.RoomID,
		Timestamp: event.Timestamp,
		Payload:   event.Payload,
	})
	if err != nil {
		return
	}
	r.hub(event.RoomID).broadcast <- payload
}
