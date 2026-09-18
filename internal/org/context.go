package org

import (
	"context"

	"github.com/google/uuid"
)

// membershipKey is the unexported key for the caller's membership in context.
// One key holding the whole membership, so role travels wherever org id does.
type membershipKey struct{}

// WithMembership returns a context carrying the caller's membership in the
// organization this request addressed. Set by middleware.RequireOrg only.
func WithMembership(ctx context.Context, m *Membership) context.Context {
	return context.WithValue(ctx, membershipKey{}, m)
}

// CurrentMembership returns the membership RequireOrg stored in ctx.
//
// This is the only accessor. It is fallible on purpose: an infallible variant
// beside it would rebuild the silent uuid.Nil scoping bug with the shorter
// name winning at 3pm.
func CurrentMembership(ctx context.Context) (*Membership, error) {
	m, ok := ctx.Value(membershipKey{}).(*Membership)
	if !ok || m == nil {
		return nil, ErrNoOrgContext
	}

	return m, nil
}

// OrgID returns the active organization for this request.
func OrgID(ctx context.Context) (uuid.UUID, error) {
	m, err := CurrentMembership(ctx)
	if err != nil {
		return uuid.Nil, err
	}

	return m.OrgID, nil
}
