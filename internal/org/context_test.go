package org_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/org"
)

// A context with no membership must fail loudly. Returning uuid.Nil instead
// would scope every query to an organization that does not exist, which reads
// as "empty organization" rather than "tenancy bug".
func TestOrgID_FailsClosed(t *testing.T) {
	t.Parallel()

	_, err := org.OrgID(context.Background())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = org.CurrentMembership(context.Background())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)
}

func TestOrgID_NilMembershipFailsClosed(t *testing.T) {
	t.Parallel()

	ctx := org.WithMembership(context.Background(), nil)

	_, err := org.OrgID(ctx)
	assert.ErrorIs(t, err, org.ErrNoOrgContext)
}

func TestCurrentMembership_RoundTrip(t *testing.T) {
	t.Parallel()

	want := &org.Membership{
		UserID: uuid.New(),
		OrgID:  uuid.New(),
		Role:   org.RoleAccountant,
	}

	ctx := org.WithMembership(context.Background(), want)

	got, err := org.CurrentMembership(ctx)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	orgID, err := org.OrgID(ctx)
	require.NoError(t, err)
	assert.Equal(t, want.OrgID, orgID)
}
