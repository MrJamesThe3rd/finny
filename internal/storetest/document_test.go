package storetest

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/audit"
	"github.com/MrJamesThe3rd/finny/internal/document"
	docstore "github.com/MrJamesThe3rd/finny/internal/document/store"
	"github.com/MrJamesThe3rd/finny/internal/org"
)

// newBackendAndDocument gives a tenant one backend, one document and one
// location — the full shape the six previously-unscoped queries touch.
func newBackendAndDocument(t *testing.T, store *docstore.Store, tn tenant) (document.BackendConfig, document.Document) {
	t.Helper()

	cfg := document.BackendConfig{
		Type:    "local",
		Name:    "primary",
		Config:  json.RawMessage(`{"base_path":"docs"}`),
		Enabled: true,
	}
	require.NoError(t, store.CreateBackend(tn.Ctx, &cfg))

	doc := document.Document{Filename: "fatura.pdf", MIMEType: "application/pdf"}
	require.NoError(t, store.CreateDocument(tn.Ctx, &doc))

	loc := document.Location{DocumentID: doc.ID, BackendID: cfg.ID, Key: "42"}
	require.NoError(t, store.AddLocation(tn.Ctx, &loc))
	require.NotEqual(t, uuid.Nil, loc.ID)

	return cfg, doc
}

// The document store is where all six missing predicates lived, so this is
// where the isolation suite is concentrated.
func TestDocumentStore_Isolation(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := docstore.New(testDB)

	orgA := newTenant(t, "Org A")
	orgB := newTenant(t, "Org B")

	cfgA, docA := newBackendAndDocument(t, store, orgA)

	t.Run("ListBackendsExcludesOtherOrgs", func(t *testing.T) {
		got, err := store.ListBackends(orgB.Ctx)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("GetBackend", func(t *testing.T) {
		_, err := store.GetBackend(orgB.Ctx, cfgA.ID)
		assert.ErrorIs(t, err, document.ErrBackendNotFound)
	})

	t.Run("UpdateBackend", func(t *testing.T) {
		name := "stolen"
		err := store.UpdateBackend(orgB.Ctx, cfgA.ID, &name, nil, nil)
		assert.ErrorIs(t, err, document.ErrBackendNotFound)

		// And nothing changed for the owner.
		got, err := store.GetBackend(orgA.Ctx, cfgA.ID)
		require.NoError(t, err)
		assert.Equal(t, "primary", got.Name)
	})

	t.Run("DeleteBackend", func(t *testing.T) {
		err := store.DeleteBackend(orgB.Ctx, cfgA.ID)
		assert.ErrorIs(t, err, document.ErrBackendNotFound)

		_, err = store.GetBackend(orgA.Ctx, cfgA.ID)
		require.NoError(t, err)
	})

	t.Run("BackendHasDocuments", func(t *testing.T) {
		got, err := store.BackendHasDocuments(orgB.Ctx, cfgA.ID)
		require.NoError(t, err)
		assert.False(t, got, "org B must not learn that org A's backend holds documents")

		got, err = store.BackendHasDocuments(orgA.Ctx, cfgA.ID)
		require.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("GetDocument", func(t *testing.T) {
		_, err := store.GetDocument(orgB.Ctx, docA.ID)
		assert.ErrorIs(t, err, document.ErrDocumentNotFound)
	})

	t.Run("ListLocations", func(t *testing.T) {
		got, err := store.ListLocations(orgB.Ctx, docA.ID)
		require.NoError(t, err)
		assert.Empty(t, got, "locations are scoped through documents, not their own column")

		got, err = store.ListLocations(orgA.Ctx, docA.ID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("AddLocation", func(t *testing.T) {
		loc := document.Location{DocumentID: docA.ID, BackendID: cfgA.ID, Key: "99"}
		err := store.AddLocation(orgB.Ctx, &loc)
		assert.ErrorIs(t, err, document.ErrDocumentNotFound)
	})

	t.Run("DeleteDocument", func(t *testing.T) {
		err := store.DeleteDocument(orgB.Ctx, docA.ID)
		assert.ErrorIs(t, err, document.ErrDocumentNotFound)

		_, err = store.GetDocument(orgA.Ctx, docA.ID)
		require.NoError(t, err, "org A's document must survive org B's delete")
	})

	t.Run("PurgeDocument", func(t *testing.T) {
		require.NoError(t, store.PurgeDocument(orgB.Ctx, docA.ID))

		_, err := store.GetDocument(orgA.Ctx, docA.ID)
		require.NoError(t, err, "the unaudited cleanup path is scoped too")
	})
}

// Every store method must fail loudly on a context with no membership. An
// empty result set would read as "empty organization" rather than a tenancy bug.
func TestDocumentStore_BareContextFailsClosed(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := docstore.New(testDB)
	ctx := context.Background()

	_, err := store.ListBackends(ctx)
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = store.ListEnabledBackends(ctx)
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = store.GetBackend(ctx, uuid.New())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = store.GetDocument(ctx, uuid.New())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = store.ListLocations(ctx, uuid.New())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = store.BackendHasDocuments(ctx, uuid.New())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	assert.ErrorIs(t, store.CreateDocument(ctx, &document.Document{}), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.CreateBackend(ctx, &document.BackendConfig{}), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.DeleteDocument(ctx, uuid.New()), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.PurgeDocument(ctx, uuid.New()), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.DeleteBackend(ctx, uuid.New()), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.AddLocation(ctx, &document.Location{}), org.ErrNoOrgContext)
}

func TestDocumentStore_ListEnabledBackendsExcludesDisabled(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := docstore.New(testDB)
	tn := newTenant(t, "Enabled Filter")

	enabled := document.BackendConfig{Type: "local", Name: "on", Config: json.RawMessage(`{}`), Enabled: true}
	require.NoError(t, store.CreateBackend(tn.Ctx, &enabled))

	disabled := document.BackendConfig{Type: "local", Name: "off", Config: json.RawMessage(`{}`), Enabled: false}
	require.NoError(t, store.CreateBackend(tn.Ctx, &disabled))

	all, err := store.ListBackends(tn.Ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2, "the settings UI must still see disabled backends")

	got, err := store.ListEnabledBackends(tn.Ctx)
	require.NoError(t, err)
	require.Len(t, got, 1, "the upload path must never write to a disabled backend")
	assert.Equal(t, enabled.ID, got[0].ID)
}

// The upload rollback must not write a delete row: it would be
// indistinguishable from someone deliberately destroying a document.
func TestDocumentStore_PurgeIsUnaudited(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := docstore.New(testDB)
	tn := newTenant(t, "Purge Audit")

	doc := document.Document{Filename: "partial.pdf", MIMEType: "application/pdf"}
	require.NoError(t, store.CreateDocument(tn.Ctx, &doc))
	assert.Equal(t, 1, auditCount(t, tn.OrgID, audit.SubjectDocument, doc.ID), "creation is audited")

	require.NoError(t, store.PurgeDocument(tn.Ctx, doc.ID))
	assert.Equal(t, 1, auditCount(t, tn.OrgID, audit.SubjectDocument, doc.ID),
		"rollback cleanup must not add a delete row")

	action, _, _, _ := lastAudit(t, tn.OrgID, audit.SubjectDocument, doc.ID)
	assert.Equal(t, string(audit.ActionCreate), action)
}

// Backend credentials live in the config JSONB. audit_log is read far more
// widely than document_backends, so the config must never be copied into it.
func TestDocumentStore_AuditOmitsBackendConfig(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := docstore.New(testDB)
	tn := newTenant(t, "Backend Audit")

	cfg := document.BackendConfig{
		Type:    "paperless",
		Name:    "paperless",
		Config:  json.RawMessage(`{"base_url":"https://paperless.example.com","token":"super-secret-token"}`),
		Enabled: true,
	}
	require.NoError(t, store.CreateBackend(tn.Ctx, &cfg))

	_, actor, _, newValue := lastAudit(t, tn.OrgID, audit.SubjectBackend, cfg.ID)
	assert.Equal(t, tn.UserID, actor)
	assert.NotContains(t, string(newValue), "super-secret-token")
	assert.NotContains(t, string(newValue), "base_url")
	assert.Contains(t, string(newValue), "paperless")
}
