package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

//go:generate mockgen -source=service.go -destination=repository_mock.go -package=auth
type Repository interface {
	// User operations
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	// GetUserByLogin finds a user matching login as email OR username.
	GetUserByLogin(ctx context.Context, login string) (*User, error)
	ListUsers(ctx context.Context) ([]*User, error)
	DeleteUser(ctx context.Context, id uuid.UUID) error
	UpdateLastLogin(ctx context.Context, id uuid.UUID) error

	// Refresh token operations
	CreateRefreshToken(ctx context.Context, token *RefreshToken) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error
}

// Service handles all authentication and user management logic.
type Service struct {
	repo          Repository
	secret        string
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewService creates a Service. secret must be the raw JWT signing secret string.
func NewService(repo Repository, secret string, accessExpiry, refreshExpiry time.Duration) *Service {
	return &Service{
		repo:          repo,
		secret:        secret,
		accessExpiry:  accessExpiry,
		refreshExpiry: refreshExpiry,
	}
}

// LoginResult holds the tokens returned after a successful login or refresh.
type LoginResult struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// Login authenticates a user by email or username + password.
// Returns ErrInvalidCredentials for any auth failure — never reveals which field is wrong.
func (s *Service) Login(ctx context.Context, login, password string) (*LoginResult, error) {
	user, err := s.repo.GetUserByLogin(ctx, login)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.issueTokenPair(ctx, user)
}

// Refresh validates the raw refresh token, revokes it, and issues a new token pair.
func (s *Service) Refresh(ctx context.Context, rawRefreshToken string) (*LoginResult, error) {
	hash := HashToken(rawRefreshToken)

	rt, err := s.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, fmt.Errorf("get refresh token: %w", err)
	}

	if rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		return nil, ErrTokenInvalid
	}

	user, err := s.repo.GetUserByID(ctx, rt.UserID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	if err := s.repo.RevokeRefreshToken(ctx, hash); err != nil {
		// ErrTokenNotFound means a concurrent request already revoked it — treat as invalid.
		if errors.Is(err, ErrTokenNotFound) {
			return nil, ErrTokenInvalid
		}
		return nil, fmt.Errorf("revoke token: %w", err)
	}

	return s.issueTokenPair(ctx, user)
}

// Logout revokes the given raw refresh token.
// Treats already-revoked tokens as success — logout is idempotent.
func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	err := s.repo.RevokeRefreshToken(ctx, HashToken(rawRefreshToken))
	if err != nil && !errors.Is(err, ErrTokenNotFound) {
		return fmt.Errorf("revoke token: %w", err)
	}
	return nil
}

// CreateUserParams holds the inputs for creating a new user.
type CreateUserParams struct {
	Email    string
	Username string
	Name     string
	Password string
	IsAdmin  bool
}

// CreateUser creates a new user, hashing their password with bcrypt cost 12.
func (s *Service) CreateUser(ctx context.Context, params CreateUserParams) (*User, error) {
	if len(params.Password) < 8 {
		return nil, ErrPasswordTooShort
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(params.Password), 12)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	now := time.Now()
	user := &User{
		ID:           uuid.New(),
		Email:        params.Email,
		Username:     params.Username,
		Name:         params.Name,
		PasswordHash: string(hash),
		IsAdmin:      params.IsAdmin,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

// ListUsers returns all users.
func (s *Service) ListUsers(ctx context.Context) ([]*User, error) {
	return s.repo.ListUsers(ctx)
}

// DeleteUser removes a user by ID.
func (s *Service) DeleteUser(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteUser(ctx, id)
}

// issueTokenPair creates a new access + refresh token for the given user.
func (s *Service) issueTokenPair(ctx context.Context, user *User) (*LoginResult, error) {
	accessToken, err := SignAccessToken(user, s.secret, s.accessExpiry)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	rawRefresh, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	now := time.Now()
	rt := &RefreshToken{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: HashToken(rawRefresh),
		ExpiresAt: now.Add(s.refreshExpiry),
		CreatedAt: now,
	}
	if err := s.repo.CreateRefreshToken(ctx, rt); err != nil {
		return nil, fmt.Errorf("store refresh token: %w", err)
	}

	_ = s.repo.UpdateLastLogin(ctx, user.ID) // best-effort; don't fail login on this

	return &LoginResult{
		AccessToken:  accessToken,
		RefreshToken: rawRefresh,
		ExpiresAt:    now.Add(s.accessExpiry),
	}, nil
}
