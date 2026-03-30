package main

import (
	"context"
	"fmt"

	"github.com/Ci7amka/metachat/internal/models"
)

type Service struct {
	repo *Repository
	hub  *Hub
}

func NewService(repo *Repository, hub *Hub) *Service {
	return &Service{repo: repo, hub: hub}
}

func (s *Service) CreateConversation(ctx context.Context, userID string, req *models.CreateConversationRequest) (*models.Conversation, error) {
	// Ensure the creator is a participant
	allParticipants := append(req.ParticipantIDs, userID)
	seen := make(map[string]bool)
	unique := []string{}
	for _, id := range allParticipants {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	if len(unique) < 2 {
		return nil, fmt.Errorf("conversation requires at least 2 participants")
	}

	// For direct (2-person) conversations, check if one already exists
	if len(unique) == 2 {
		existing, err := s.repo.FindDirectConversation(ctx, unique[0], unique[1])
		if err != nil {
			return nil, fmt.Errorf("check existing conversation: %w", err)
		}
		if existing != nil {
			return existing, nil
		}
	}

	return s.repo.CreateConversation(ctx, unique)
}

func (s *Service) GetConversations(ctx context.Context, userID string) ([]models.Conversation, error) {
	return s.repo.GetUserConversations(ctx, userID)
}

func (s *Service) GetMessages(ctx context.Context, userID, conversationID string, limit int, before string) ([]models.Message, error) {
	ok, err := s.repo.IsParticipant(ctx, conversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("check participant: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("not a participant in this conversation")
	}

	return s.repo.GetMessages(ctx, conversationID, limit, before)
}

func (s *Service) SendMessage(ctx context.Context, userID string, req *models.SendMessageRequest) (*models.Message, error) {
	ok, err := s.repo.IsParticipant(ctx, req.ConversationID, userID)
	if err != nil {
		return nil, fmt.Errorf("check participant: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("not a participant in this conversation")
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "text"
	}

	msg := &models.Message{
		ConversationID: req.ConversationID,
		SenderID:       userID,
		Content:        req.Content,
		ContentType:    contentType,
	}

	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	// Fan out to WebSocket connections
	participants, err := s.repo.GetConversationParticipants(ctx, req.ConversationID)
	if err == nil {
		event := models.WSEvent{
			Type:    "message",
			Payload: msg,
		}
		for _, pid := range participants {
			s.hub.SendToUser(pid, event)
		}
	}

	return msg, nil
}

func (s *Service) MarkRead(ctx context.Context, userID, messageID, conversationID string) error {
	ok, err := s.repo.IsParticipant(ctx, conversationID, userID)
	if err != nil {
		return fmt.Errorf("check participant: %w", err)
	}
	if !ok {
		return fmt.Errorf("not a participant in this conversation")
	}

	if err := s.repo.MarkMessageRead(ctx, messageID, userID); err != nil {
		return fmt.Errorf("mark read: %w", err)
	}

	// Notify sender about read receipt
	participants, err := s.repo.GetConversationParticipants(ctx, conversationID)
	if err == nil {
		event := models.WSEvent{
			Type: "read_receipt",
			Payload: models.WSReadReceiptPayload{
				ConversationID: conversationID,
				MessageID:      messageID,
				UserID:         userID,
			},
		}
		for _, pid := range participants {
			s.hub.SendToUser(pid, event)
		}
	}

	return nil
}

func (s *Service) HandleTyping(userID, conversationID string, isTyping bool) {
	participants, err := s.repo.GetConversationParticipants(context.Background(), conversationID)
	if err != nil {
		return
	}

	event := models.WSEvent{
		Type: "typing",
		Payload: models.WSTypingPayload{
			ConversationID: conversationID,
			UserID:         userID,
			IsTyping:       isTyping,
		},
	}

	for _, pid := range participants {
		if pid != userID {
			s.hub.SendToUser(pid, event)
		}
	}
}
