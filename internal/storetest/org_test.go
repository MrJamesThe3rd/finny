package storetest

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/audit"
	"github.com/MrJamesThe3rd/finny/internal/auth"
	"github.com/MrJamesThe3rd/finny/internal/org"
	orgstore "github.com/MrJamesThe3rd/finny/internal/org/store"
)

func TestOrgStore_ListForUser(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := orgstore.New(testDB)

	orgA := newTenant(t, "Practice Client One")
	orgB := newTenant(t, "Practice Client Two")

	got, err := store.ListForUser(context.Background(), orgA.UserID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, orgA.OrgID, got[0].ID)

	// An accountant holds memberships in several organizations.
	accountant := insertUser(t)
	require.NoError(t, store.AddMember(orgA.Ctx, &org.Membership{
		UserID: accountant, OrgID: orgA.OrgID, Role: org.RoleAccountant,
	}))
	require.NoError(t, store.AddMember(orgB.Ctx, &org.Membership{
		UserID: accountant, OrgID: orgB.OrgID, Role: org.RoleAccountant,
	}))

	got, err = store.ListForUser(context.Background(), accountant)
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestOrgStore_GetMembership(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := orgstore.New(testDB)
	tn := newTenant(t, "Membership Lookup")
	ctx := context.Background()

	t.Run("Member", func(t *testing.T) {
		got, err := store.GetMembership(ctx, tn.UserID, tn.OrgID)
		require.NoError(t, err)
		assert.Equal(t, org.RoleOwner, got.Role)
	})

	t.Run("StrangerGetsTheSameAnswerAsAMissingOrg", func(t *testing.T) {
		stranger := insertUser(t)

		_, err := store.GetMembership(ctx, stranger, tn.OrgID)
		assert.ErrorIs(t, err, org.ErrNotFound)

		_, err = store.GetMembership(ctx, tn.UserID, uuid.New())
		assert.ErrorIs(t, err, org.ErrNotFound)
	})

	t.Run("RevokedIsNotAMember", func(t *testing.T) {
		accountant := insertUser(t)
		require.NoError(t, store.AddMember(tn.Ctx, &org.Membership{
			UserID: accountant, OrgID: tn.OrgID, Role: org.RoleAccountant,
		}))

		_, err := store.GetMembership(ctx, accountant, tn.OrgID)
		require.NoError(t, err)

		require.NoError(t, store.RevokeMember(tn.Ctx, tn.OrgID, accountant))

		_, err = store.GetMembership(ctx, accountant, tn.OrgID)
		assert.ErrorIs(t, err, org.ErrNotFound, "a revoked member is refused on the next request")

		// Append-only: the row survives with revoked_at set, because it is the
		// record that someone had access during a period they signed off.
		var revoked bool
		require.NoError(t, testDB.QueryRowContext(ctx, `
			SELECT revoked_at IS NOT NULL FROM memberships WHERE org_id = $1 AND user_id = $2
		`, tn.OrgID, accountant).Scan(&revoked))
		assert.True(t, revoked)
	})
}

// An organization that loses its last owner permanently 403s its own
// owner-only routes, so the store refuses.
func TestOrgStore_RevokeMember_LastOwner(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := orgstore.New(testDB)
	tn := newTenant(t, "Last Owner")

	err := store.RevokeMember(tn.Ctx, tn.OrgID, tn.UserID)
	assert.ErrorIs(t, err, org.ErrLastOwner)

	// With a second owner it is allowed, and the org keeps one.
	second := insertUser(t)
	require.NoError(t, store.AddMember(tn.Ctx, &org.Membership{
		UserID: second, OrgID: tn.OrgID, Role: org.RoleOwner,
	}))

	require.NoError(t, store.RevokeMember(tn.Ctx, tn.OrgID, tn.UserID))

	_, err = store.GetMembership(context.Background(), second, tn.OrgID)
	require.NoError(t, err)

	// And now the survivor is the last owner.
	assert.ErrorIs(t, store.RevokeMember(tn.Ctx, tn.OrgID, second), org.ErrLastOwner)
}

func TestOrgStore_RevokeMember_NotAMember(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := orgstore.New(testDB)
	tn := newTenant(t, "Revoke Stranger")

	err := store.RevokeMember(tn.Ctx, tn.OrgID, insertUser(t))
	assert.ErrorIs(t, err, org.ErrNotFound)
}

// Re-granting a revoked membership reactivates the one row: memberships is
// UNIQUE (user_id, org_id), so a second row cannot exist.
func TestOrgStore_AddMember_ReactivatesRevoked(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := orgstore.New(testDB)
	tn := newTenant(t, "Reactivate")
	accountant := insertUser(t)

	require.NoError(t, store.AddMember(tn.Ctx, &org.Membership{
		UserID: accountant, OrgID: tn.OrgID, Role: org.RoleAccountant,
	}))
	require.NoError(t, store.RevokeMember(tn.Ctx, tn.OrgID, accountant))
	require.NoError(t, store.AddMember(tn.Ctx, &org.Membership{
		UserID: accountant, OrgID: tn.OrgID, Role: org.RoleAccountant,
	}))

	got, err := store.GetMembership(context.Background(), accountant, tn.OrgID)
	require.NoError(t, err)
	assert.Equal(t, org.RoleAccountant, got.Role)

	var rows int
	require.NoError(t, testDB.QueryRowContext(context.Background(), `
		SELECT count(*) FROM memberships WHERE org_id = $1 AND user_id = $2
	`, tn.OrgID, accountant).Scan(&rows))
	assert.Equal(t, 1, rows)
}

// An organization with no owner is unreachable, so both rows commit together.
func TestOrgStore_CreateWithOwnerIsAudited(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := orgstore.New(testDB)
	userID := insertUser(t)
	ctx := auth.WithUserID(context.Background(), userID)

	o := &org.Organization{Name: "Audited Org", NIF: "517948974"}
	require.NoError(t, store.CreateWithOwner(ctx, o, userID))

	assert.Equal(t, 1, auditCount(t, o.ID, audit.SubjectOrganization, o.ID))
	assert.Equal(t, 1, auditCount(t, o.ID, audit.SubjectMembership, userID))

	action, actor, _, _ := lastAudit(t, o.ID, audit.SubjectMembership, userID)
	assert.Equal(t, string(audit.ActionGrant), action)
	assert.Equal(t, userID, actor)
}

func TestOrgStore_MembershipGrantAndRevokeAreAudited(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := orgstore.New(testDB)
	tn := newTenant(t, "Member Audit")
	accountant := insertUser(t)

	require.NoError(t, store.AddMember(tn.Ctx, &org.Membership{
		UserID: accountant, OrgID: tn.OrgID, Role: org.RoleAccountant,
	}))
	require.NoError(t, store.RevokeMember(tn.Ctx, tn.OrgID, accountant))

	assert.Equal(t, 2, auditCount(t, tn.OrgID, audit.SubjectMembership, accountant))

	action, actor, oldValue, _ := lastAudit(t, tn.OrgID, audit.SubjectMembership, accountant)
	assert.Equal(t, string(audit.ActionRevoke), action)
	assert.Equal(t, tn.UserID, actor, "the actor is the owner who revoked, not the member")
	assert.Contains(t, string(oldValue), string(org.RoleAccountant))
}
