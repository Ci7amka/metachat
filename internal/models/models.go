package models

import "time"

// User represents a user in the system.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	Name         string    `json:"name"`
	Bio          string    `json:"bio"`
	Age          *int      `json:"age,omitempty"`
	Interests    []string  `json:"interests"`
	AvatarURL    string    `json:"avatar_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Conversation represents a chat conversation.
type Conversation struct {
	ID            string    `json:"id"`
	Participants  []string  `json:"participants,omitempty"`
	LastMessage   *Message  `json:"last_message,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Message represents a chat message.
type Message struct {
	ID             string     `json:"id"`
	ConversationID string     `json:"conversation_id"`
	SenderID       string     `json:"sender_id"`
	Content        string     `json:"content"`
	ContentType    string     `json:"content_type"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// RefreshToken represents a stored refresh token.
type RefreshToken struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	TokenHash string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// --- Request/Response types ---

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         *User  `json:"user,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type UpdateProfileRequest struct {
	Name      *string  `json:"name,omitempty"`
	Bio       *string  `json:"bio,omitempty"`
	Age       *int     `json:"age,omitempty"`
	Interests []string `json:"interests,omitempty"`
	AvatarURL *string  `json:"avatar_url,omitempty"`
}

type ValidateTokenRequest struct {
	Token string `json:"token"`
}

type ValidateTokenResponse struct {
	Valid  bool   `json:"valid"`
	UserID string `json:"user_id,omitempty"`
}

type SendMessageRequest struct {
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	ContentType    string `json:"content_type,omitempty"`
}

type CreateConversationRequest struct {
	ParticipantIDs []string `json:"participant_ids"`
}

type GetMessagesRequest struct {
	ConversationID string `json:"conversation_id"`
	Limit          int    `json:"limit,omitempty"`
	Before         string `json:"before,omitempty"` // cursor: message ID
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// WebSocket event types
type WSEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type WSMessagePayload struct {
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
	ContentType    string `json:"content_type,omitempty"`
}

type WSTypingPayload struct {
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	IsTyping       bool   `json:"is_typing"`
}

type WSReadReceiptPayload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	UserID         string `json:"user_id"`
}
