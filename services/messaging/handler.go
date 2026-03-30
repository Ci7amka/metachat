package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Ci7amka/metachat/internal/models"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for MVP
	},
}

type Handler struct {
	svc *Service
	hub *Hub
}

func NewHandler(svc *Service, hub *Hub) *Handler {
	return &Handler{svc: svc, hub: hub}
}

func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade error", "error", err)
		return
	}

	client := &Client{
		hub:    h.hub,
		userID: userID,
		conn:   conn,
		send:   make(chan []byte, 256),
	}

	h.hub.Register(client)

	go client.writePump()
	go client.readPump(h.svc)
}

func (h *Handler) GetConversations(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing user ID")
		return
	}

	conversations, err := h.svc.GetConversations(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if conversations == nil {
		conversations = []models.Conversation{}
	}

	writeJSON(w, http.StatusOK, conversations)
}

func (h *Handler) CreateConversation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing user ID")
		return
	}

	var req models.CreateConversationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	conv, err := h.svc.CreateConversation(r.Context(), userID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, conv)
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing user ID")
		return
	}

	conversationID := r.URL.Query().Get("conversation_id")
	if conversationID == "" {
		writeError(w, http.StatusBadRequest, "conversation_id is required")
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	before := r.URL.Query().Get("before")

	messages, err := h.svc.GetMessages(r.Context(), userID, conversationID, limit, before)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if messages == nil {
		messages = []models.Message{}
	}

	writeJSON(w, http.StatusOK, messages)
}

func (h *Handler) SendMessage(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing user ID")
		return
	}

	var req models.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	msg, err := h.svc.SendMessage(context.Background(), userID, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, msg)
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "missing user ID")
		return
	}

	var req struct {
		MessageID      string `json:"message_id"`
		ConversationID string `json:"conversation_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.svc.MarkRead(r.Context(), userID, req.MessageID, req.ConversationID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) GetOnlineUsers(w http.ResponseWriter, r *http.Request) {
	users := h.hub.GetOnlineUsers()
	writeJSON(w, http.StatusOK, map[string]interface{}{"online_users": users})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, models.ErrorResponse{Error: msg})
}
