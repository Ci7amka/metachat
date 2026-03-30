package main

import (
	"context"
	"time"

	"github.com/Ci7amka/metachat/internal/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateConversation(ctx context.Context, participantIDs []string) (*models.Conversation, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var conv models.Conversation
	err = tx.QueryRow(ctx,
		`INSERT INTO conversations DEFAULT VALUES RETURNING id, created_at, updated_at`,
	).Scan(&conv.ID, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		return nil, err
	}

	for _, uid := range participantIDs {
		_, err = tx.Exec(ctx,
			`INSERT INTO conversation_participants (conversation_id, user_id) VALUES ($1, $2)`,
			conv.ID, uid,
		)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	conv.Participants = participantIDs
	return &conv, nil
}

func (r *Repository) GetConversation(ctx context.Context, conversationID string) (*models.Conversation, error) {
	conv := &models.Conversation{}
	err := r.db.QueryRow(ctx,
		`SELECT id, created_at, updated_at FROM conversations WHERE id = $1`,
		conversationID,
	).Scan(&conv.ID, &conv.CreatedAt, &conv.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	rows, err := r.db.Query(ctx,
		`SELECT user_id FROM conversation_participants WHERE conversation_id = $1`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		conv.Participants = append(conv.Participants, uid)
	}

	return conv, nil
}

func (r *Repository) GetUserConversations(ctx context.Context, userID string) ([]models.Conversation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.created_at, c.updated_at
		FROM conversations c
		JOIN conversation_participants cp ON c.id = cp.conversation_id
		WHERE cp.user_id = $1
		ORDER BY c.updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conversations []models.Conversation
	for rows.Next() {
		var conv models.Conversation
		if err := rows.Scan(&conv.ID, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, err
		}
		conversations = append(conversations, conv)
	}

	// Load participants and last message for each conversation
	for i := range conversations {
		pRows, err := r.db.Query(ctx,
			`SELECT user_id FROM conversation_participants WHERE conversation_id = $1`,
			conversations[i].ID,
		)
		if err != nil {
			return nil, err
		}
		for pRows.Next() {
			var uid string
			if err := pRows.Scan(&uid); err != nil {
				pRows.Close()
				return nil, err
			}
			conversations[i].Participants = append(conversations[i].Participants, uid)
		}
		pRows.Close()

		// Get last message
		var msg models.Message
		err = r.db.QueryRow(ctx, `
			SELECT id, conversation_id, sender_id, content, content_type, read_at, created_at
			FROM messages WHERE conversation_id = $1
			ORDER BY created_at DESC LIMIT 1`,
			conversations[i].ID,
		).Scan(&msg.ID, &msg.ConversationID, &msg.SenderID, &msg.Content,
			&msg.ContentType, &msg.ReadAt, &msg.CreatedAt)
		if err == nil {
			conversations[i].LastMessage = &msg
		}
	}

	return conversations, nil
}

func (r *Repository) IsParticipant(ctx context.Context, conversationID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM conversation_participants WHERE conversation_id = $1 AND user_id = $2)`,
		conversationID, userID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) FindDirectConversation(ctx context.Context, userID1, userID2 string) (*models.Conversation, error) {
	var convID string
	err := r.db.QueryRow(ctx, `
		SELECT cp1.conversation_id FROM conversation_participants cp1
		JOIN conversation_participants cp2 ON cp1.conversation_id = cp2.conversation_id
		WHERE cp1.user_id = $1 AND cp2.user_id = $2
		AND (SELECT COUNT(*) FROM conversation_participants WHERE conversation_id = cp1.conversation_id) = 2
		LIMIT 1`,
		userID1, userID2,
	).Scan(&convID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return r.GetConversation(ctx, convID)
}

func (r *Repository) CreateMessage(ctx context.Context, msg *models.Message) error {
	err := r.db.QueryRow(ctx, `
		INSERT INTO messages (conversation_id, sender_id, content, content_type)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		msg.ConversationID, msg.SenderID, msg.Content, msg.ContentType,
	).Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return err
	}

	// Update conversation updated_at
	_, err = r.db.Exec(ctx,
		`UPDATE conversations SET updated_at = NOW() WHERE id = $1`,
		msg.ConversationID,
	)
	return err
}

func (r *Repository) GetMessages(ctx context.Context, conversationID string, limit int, before string) ([]models.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var rows pgx.Rows
	var err error

	if before != "" {
		var beforeTime time.Time
		err = r.db.QueryRow(ctx,
			`SELECT created_at FROM messages WHERE id = $1`, before,
		).Scan(&beforeTime)
		if err != nil {
			return nil, err
		}

		rows, err = r.db.Query(ctx, `
			SELECT id, conversation_id, sender_id, content, content_type, read_at, created_at
			FROM messages WHERE conversation_id = $1 AND created_at < $2
			ORDER BY created_at DESC LIMIT $3`,
			conversationID, beforeTime, limit,
		)
	} else {
		rows, err = r.db.Query(ctx, `
			SELECT id, conversation_id, sender_id, content, content_type, read_at, created_at
			FROM messages WHERE conversation_id = $1
			ORDER BY created_at DESC LIMIT $2`,
			conversationID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var msg models.Message
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.SenderID,
			&msg.Content, &msg.ContentType, &msg.ReadAt, &msg.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

func (r *Repository) MarkMessageRead(ctx context.Context, messageID, userID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE messages SET read_at = NOW() WHERE id = $1 AND sender_id != $2 AND read_at IS NULL`,
		messageID, userID,
	)
	return err
}

func (r *Repository) GetConversationParticipants(ctx context.Context, conversationID string) ([]string, error) {
	rows, err := r.db.Query(ctx,
		`SELECT user_id FROM conversation_participants WHERE conversation_id = $1`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		participants = append(participants, uid)
	}
	return participants, nil
}
