package auth

import (
	"context"

	"github.com/google/uuid"
)

// DefaultUserID is the well-known ID of the single hardcoded user used until
// real authentication (Phase 3) is wired up.
var DefaultUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type ctxKey struct{}

// WithUserID returns a context carrying the given user ID.
func WithUserID(ctx context.Context, userID uuid.UUID) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

// UserID extracts the user ID from ctx. Returns uuid.Nil if none is set.
func UserID(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxKey{}).(uuid.UUID)
	return id
}
