package ws

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingEvery  = 30 * time.Second
	maxMessage = 1024
)

type Client struct {
	conn      *websocket.Conn
	send      chan []byte
	roomID    uuid.UUID
	sessionID uuid.UUID
	closed    chan struct{}
	closeOnce sync.Once
}

type readConnection interface {
	SetReadLimit(int64)
	SetReadDeadline(time.Time) error
	SetPongHandler(func(string) error)
}

type writeConnection interface {
	SetWriteDeadline(time.Time) error
	WriteMessage(int, []byte) error
}

func newClient(conn *websocket.Conn, roomID, sessionID uuid.UUID, buffer int) *Client {
	return &Client{
		conn: conn, send: make(chan []byte, buffer),
		roomID: roomID, sessionID: sessionID, closed: make(chan struct{}),
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		if c.conn != nil {
			_ = c.conn.Close()
		}
		close(c.send)
		if c.closed != nil {
			close(c.closed)
		}
	})
}

func (c *Client) readPump(registry *Registry) {
	defer registry.Unregister(c.roomID, c)
	if err := configureRead(c.conn); err != nil {
		return
	}
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func configureRead(conn readConnection) error {
	conn.SetReadLimit(maxMessage)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		return err
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	return nil
}

func writeWithDeadline(conn writeConnection, messageType int, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, payload)
}

func (c *Client) writePump(registry *Registry) {
	ticker := time.NewTicker(pingEvery)
	defer ticker.Stop()
	defer registry.Unregister(c.roomID, c)
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				return
			}
			if err := writeWithDeadline(c.conn, websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := writeWithDeadline(c.conn, websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
