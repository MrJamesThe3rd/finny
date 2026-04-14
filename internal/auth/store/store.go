package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/auth"
)

// Store is the Postgres implementation of auth.Repository.
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) CreateUser(ctx context.Context, user *auth.User) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, email, username, name, password_hash, is_admin, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, user.ID, user.Email, user.Username, user.Name, user.PasswordHash, user.IsAdmin, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating user: %w", err)
	}
	return nil
}

func (s *Store) GetUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error) {
	var u auth.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, COALESCE(username, ''), COALESCE(name, ''), COALESCE(password_hash, ''),
		       is_admin, created_at, updated_at, last_login_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Username, &u.Name, &u.PasswordHash,
		&u.IsAdmin, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, auth.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting user by id: %w", err)
	}
	return &u, nil
}

func (s *Store) GetUserByLogin(ctx context.Context, login string) (*auth.User, error) {
	var u auth.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, COALESCE(username, ''), COALESCE(name, ''), COALESCE(password_hash, ''),
		       is_admin, created_at, updated_at, last_login_at
		FROM users WHERE email = $1 OR username = $1
		LIMIT 1
	`, login).Scan(&u.ID, &u.Email, &u.Username, &u.Name, &u.PasswordHash,
		&u.IsAdmin, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, auth.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting user by login: %w", err)
	}
	return &u, nil
}

// ListUsers returns all users ordered by creation date.
// Note: PasswordHash is intentionally not populated in list results.
func (s *Store) ListUsers(ctx context.Context) ([]*auth.User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, email, COALESCE(username, ''), COALESCE(name, ''),
		       is_admin, created_at, updated_at, last_login_at
		FROM users ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer rows.Close()

	var users []*auth.User
	for rows.Next() {
		var u auth.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Username, &u.Name,
			&u.IsAdmin, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt); err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		users = append(users, &u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating users: %w", err)
	}
	return users, nil
}

func (s *Store) DeleteUser(ctx context.Context, id uuid.UUID) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return auth.ErrUserNotFound
	}
	return nil
}

func (s *Store) UpdateLastLogin(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("updating last login: %w", err)
	}
	return nil
}

func (s *Store) CreateRefreshToken(ctx context.Context, token *auth.RefreshToken) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating refresh token: %w", err)
	}
	return nil
}

func (s *Store) GetRefreshToken(ctx context.Context, tokenHash string) (*auth.RefreshToken, error) {
	var t auth.RefreshToken
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.ExpiresAt, &t.RevokedAt, &t.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, auth.ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting refresh token: %w", err)
	}
	return &t, nil
}

func (s *Store) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW()
		WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash)
	if err != nil {
		return fmt.Errorf("revoking refresh token: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return auth.ErrTokenNotFound
	}
	return nil
}

func (s *Store) RevokeAllRefreshTokens(ctx context.Context, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE refresh_tokens SET revoked_at = NOW()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("revoking all refresh tokens: %w", err)
	}
	return nil
}
