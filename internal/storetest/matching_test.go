package storetest

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	matchingstore "github.com/MrJamesThe3rd/finny/internal/matching/store"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

func TestMatchingStore_Isolation(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := matchingstore.New(testDB)

	orgA := newTenant(t, "Match Org A")
	orgB := newTenant(t, "Match Org B")

	require.NoError(t, store.CreateMapping(orgA.Ctx, "COMPRAS C.DEB UBER", "Uber"))

	t.Run("OtherOrgGetsNoSuggestion", func(t *testing.T) {
		got, err := store.FindMatch(orgB.Ctx, "COMPRAS C.DEB UBER 123")
		require.NoError(t, err)
		assert.Empty(t, got, "learned merchant cleanups are one org's work product")
	})

	t.Run("OwnerStillMatches", func(t *testing.T) {
		got, err := store.FindMatch(orgA.Ctx, "COMPRAS C.DEB UBER 123")
		require.NoError(t, err)
		assert.Equal(t, "Uber", got)
	})

	// The unique constraint moved from (user_id, raw_pattern) to
	// (org_id, raw_pattern), so the same pattern must be learnable twice.
	t.Run("SamePatternInBothOrgs", func(t *testing.T) {
		require.NoError(t, store.CreateMapping(orgB.Ctx, "COMPRAS C.DEB UBER", "Uber Eats"))

		gotB, err := store.FindMatch(orgB.Ctx, "COMPRAS C.DEB UBER 123")
		require.NoError(t, err)
		assert.Equal(t, "Uber Eats", gotB)

		gotA, err := store.FindMatch(orgA.Ctx, "COMPRAS C.DEB UBER 123")
		require.NoError(t, err)
		assert.Equal(t, "Uber", gotA, "org B's mapping must not overwrite org A's")
	})
}

func TestMatchingStore_BareContextFailsClosed(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := matchingstore.New(testDB)
	ctx := context.Background()

	_, err := store.FindMatch(ctx, "anything")
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	assert.ErrorIs(t, store.CreateMapping(ctx, "raw", "preferred"), org.ErrNoOrgContext)
}
