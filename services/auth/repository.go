package main

import (
	"context"
	"fmt"
	"strings"

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

func (r *Repository) CreateUser(ctx context.Context, user *models.User) error {
	query := `
		INSERT INTO users (email, password_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, query, user.Email, user.PasswordHash, user.Name).
		Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, email, password_hash, name, bio, age, interests, avatar_url, created_at, updated_at
		FROM users WHERE email = $1`
	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Bio, &user.Age, &user.Interests, &user.AvatarURL,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id string) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, email, password_hash, name, bio, age, interests, avatar_url, created_at, updated_at
		FROM users WHERE id = $1`
	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Bio, &user.Age, &user.Interests, &user.AvatarURL,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func (r *Repository) UpdateUser(ctx context.Context, id string, req *models.UpdateProfileRequest) (*models.User, error) {
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	if req.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Bio != nil {
		setClauses = append(setClauses, fmt.Sprintf("bio = $%d", argIdx))
		args = append(args, *req.Bio)
		argIdx++
	}
	if req.Age != nil {
		setClauses = append(setClauses, fmt.Sprintf("age = $%d", argIdx))
		args = append(args, *req.Age)
		argIdx++
	}
	if req.Interests != nil {
		setClauses = append(setClauses, fmt.Sprintf("interests = $%d", argIdx))
		args = append(args, req.Interests)
		argIdx++
	}
	if req.AvatarURL != nil {
		setClauses = append(setClauses, fmt.Sprintf("avatar_url = $%d", argIdx))
		args = append(args, *req.AvatarURL)
		argIdx++
	}

	if len(setClauses) == 0 {
		return r.GetUserByID(ctx, id)
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	args = append(args, id)

	query := fmt.Sprintf(`
		UPDATE users SET %s WHERE id = $%d
		RETURNING id, email, password_hash, name, bio, age, interests, avatar_url, created_at, updated_at`,
		strings.Join(setClauses, ", "), argIdx,
	)

	user := &models.User{}
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &user.Name,
		&user.Bio, &user.Age, &user.Interests, &user.AvatarURL,
		&user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *Repository) SaveRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt interface{}) error {
	query := `INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)`
	_, err := r.db.Exec(ctx, query, userID, tokenHash, expiresAt)
	return err
}

func (r *Repository) GetRefreshToken(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	rt := &models.RefreshToken{}
	query := `SELECT id, user_id, token_hash, expires_at, created_at FROM refresh_tokens WHERE token_hash = $1`
	err := r.db.QueryRow(ctx, query, tokenHash).Scan(
		&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return rt, nil
}

func (r *Repository) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	query := `DELETE FROM refresh_tokens WHERE token_hash = $1`
	_, err := r.db.Exec(ctx, query, tokenHash)
	return err
}

func (r *Repository) DeleteUserRefreshTokens(ctx context.Context, userID string) error {
	query := `DELETE FROM refresh_tokens WHERE user_id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}
