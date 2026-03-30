package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Ci7amka/metachat/internal/middleware"
	"github.com/Ci7amka/metachat/internal/models"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	repo   *Repository
	jwtMgr *middleware.JWTManager
}

func NewService(repo *Repository, jwtMgr *middleware.JWTManager) *Service {
	return &Service{repo: repo, jwtMgr: jwtMgr}
}

func (s *Service) Register(ctx context.Context, req *models.RegisterRequest) (*models.AuthResponse, error) {
	existing, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("check existing user: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("user with this email already exists")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user := &models.User{
		Email:        req.Email,
		PasswordHash: string(hash),
		Name:         req.Name,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return s.generateTokens(ctx, user)
}

func (s *Service) Login(ctx context.Context, req *models.LoginRequest) (*models.AuthResponse, error) {
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, fmt.Errorf("invalid email or password")
	}

	return s.generateTokens(ctx, user)
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*models.AuthResponse, error) {
	userID, err := s.jwtMgr.ValidateToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	tokenHash := middleware.HashToken(refreshToken)
	stored, err := s.repo.GetRefreshToken(ctx, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	if stored == nil {
		return nil, fmt.Errorf("refresh token not found")
	}
	if time.Now().After(stored.ExpiresAt) {
		_ = s.repo.DeleteRefreshToken(ctx, tokenHash)
		return nil, fmt.Errorf("refresh token expired")
	}

	// Delete old token
	_ = s.repo.DeleteRefreshToken(ctx, tokenHash)

	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil || user == nil {
		return nil, fmt.Errorf("user not found")
	}

	return s.generateTokens(ctx, user)
}

func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	tokenHash := middleware.HashToken(refreshToken)
	return s.repo.DeleteRefreshToken(ctx, tokenHash)
}

func (s *Service) GetProfile(ctx context.Context, userID string) (*models.User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, req *models.UpdateProfileRequest) (*models.User, error) {
	return s.repo.UpdateUser(ctx, userID, req)
}

func (s *Service) ValidateToken(ctx context.Context, token string) (*models.ValidateTokenResponse, error) {
	userID, err := s.jwtMgr.ValidateAccessToken(token)
	if err != nil {
		return &models.ValidateTokenResponse{Valid: false}, nil
	}
	return &models.ValidateTokenResponse{Valid: true, UserID: userID}, nil
}

func (s *Service) generateTokens(ctx context.Context, user *models.User) (*models.AuthResponse, error) {
	accessToken, err := s.jwtMgr.GenerateAccessToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := s.jwtMgr.GenerateRefreshToken(user.ID)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	tokenHash := middleware.HashToken(refreshToken)
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if err := s.repo.SaveRefreshToken(ctx, user.ID, tokenHash, expiresAt); err != nil {
		return nil, fmt.Errorf("save refresh token: %w", err)
	}

	return &models.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}, nil
}
