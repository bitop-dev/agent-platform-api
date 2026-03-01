// Package ws provides WebSocket hub for real-time run event streaming.
package ws

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

// Event is a WebSocket message sent to subscribers.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Hub manages WebSocket connections grouped by run ID.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*websocket.Conn]bool
}

// NewHub creates a WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]map[*websocket.Conn]bool),
	}
}

// Subscribe adds a connection to a run's room.
func (h *Hub) Subscribe(runID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.rooms[runID] == nil {
		h.rooms[runID] = make(map[*websocket.Conn]bool)
	}
	h.rooms[runID][conn] = true
}

// Unsubscribe removes a connection from a run's room.
func (h *Hub) Unsubscribe(runID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if room, ok := h.rooms[runID]; ok {
		delete(room, conn)
		if len(room) == 0 {
			delete(h.rooms, runID)
		}
	}
}

// Broadcast sends an event to all connections subscribed to a run.
func (h *Hub) Broadcast(runID string, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	room, ok := h.rooms[runID]
	if !ok {
		return
	}

	data, err := json.Marshal(event)
	if err != nil {
		slog.Warn("ws marshal error", "error", err)
		return
	}

	for conn := range room {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			slog.Warn("ws write error", "error", err)
			// Connection will be cleaned up by the handler
		}
	}
}

// RoomSize returns the number of subscribers for a run.
func (h *Hub) RoomSize(runID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.rooms[runID])
}
