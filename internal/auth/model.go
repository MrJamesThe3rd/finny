package auth

import (
	"time"

	"github.com/google/uuid"
)

// User represents an authenticated user of the system.
type User struct {
	ID           uuid.UUID
	Email        string
	Username     string
	Name         string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastLoginAt  *time.Time
}

// RefreshToken represents a stored refresh token (hash only — raw token is client-side).
type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}
