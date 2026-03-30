package main

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/Ci7amka/metachat/internal/models"
	"github.com/gorilla/websocket"
)

// Client represents a single WebSocket connection.
type Client struct {
	hub    *Hub
	userID string
	conn   *websocket.Conn
	send   chan []byte
}

// Hub maintains the set of active clients and broadcasts messages.
type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*Client]bool // userID -> set of clients
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*Client]bool),
	}
}

func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[client.userID] == nil {
		h.clients[client.userID] = make(map[*Client]bool)
	}
	h.clients[client.userID][client] = true
	slog.Info("client connected", "user_id", client.userID)
}

func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if clients, ok := h.clients[client.userID]; ok {
		delete(clients, client)
		if len(clients) == 0 {
			delete(h.clients, client.userID)
		}
	}
	close(client.send)
	slog.Info("client disconnected", "user_id", client.userID)
}

func (h *Hub) SendToUser(userID string, event models.WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if clients, ok := h.clients[userID]; ok {
		for client := range clients {
			select {
			case client.send <- data:
			default:
				// Client buffer full, skip
			}
		}
	}
}

func (h *Hub) IsOnline(userID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients[userID]) > 0
}

func (h *Hub) GetOnlineUsers() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	users := make([]string, 0, len(h.clients))
	for uid := range h.clients {
		users = append(users, uid)
	}
	return users
}

// readPump pumps messages from the WebSocket connection to the hub.
func (c *Client) readPump(svc *Service) {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Error("websocket read error", "error", err)
			}
			break
		}

		var event models.WSEvent
		if err := json.Unmarshal(message, &event); err != nil {
			slog.Warn("invalid ws message", "error", err)
			continue
		}

		c.handleEvent(svc, &event)
	}
}

func (c *Client) handleEvent(svc *Service, event *models.WSEvent) {
	switch event.Type {
	case "message":
		data, _ := json.Marshal(event.Payload)
		var payload models.WSMessagePayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		req := &models.SendMessageRequest{
			ConversationID: payload.ConversationID,
			Content:        payload.Content,
			ContentType:    payload.ContentType,
		}
		_, err := svc.SendMessage(nil, c.userID, req)
		if err != nil {
			slog.Error("ws send message error", "error", err)
		}

	case "typing":
		data, _ := json.Marshal(event.Payload)
		var payload models.WSTypingPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		svc.HandleTyping(c.userID, payload.ConversationID, payload.IsTyping)

	case "read_receipt":
		data, _ := json.Marshal(event.Payload)
		var payload models.WSReadReceiptPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return
		}
		_ = svc.MarkRead(nil, c.userID, payload.MessageID, payload.ConversationID)
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	defer c.conn.Close()

	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}

	// Channel closed, send close message
	c.conn.WriteMessage(websocket.CloseMessage, []byte{})
}
