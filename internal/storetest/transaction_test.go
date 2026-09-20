package storetest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/audit"
	"github.com/MrJamesThe3rd/finny/internal/org"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
	txstore "github.com/MrJamesThe3rd/finny/internal/transaction/store"
)

func newTransaction(t *testing.T, store *txstore.Store, tn tenant) *transaction.Transaction {
	t.Helper()

	tx := &transaction.Transaction{
		Amount:         -6400,
		Type:           transaction.TypeExpense,
		Status:         transaction.StatusPendingInvoice,
		Description:    "PA Gondomar",
		RawDescription: "COMPRA PA GONDOMAR",
		Date:           time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.CreateTransaction(tn.Ctx, tx))
	require.NotEqual(t, uuid.Nil, tx.ID)

	return tx
}

func TestTransactionStore_Isolation(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := txstore.New(testDB)

	orgA := newTenant(t, "Tx Org A")
	orgB := newTenant(t, "Tx Org B")

	txA := newTransaction(t, store, orgA)

	t.Run("Get", func(t *testing.T) {
		_, err := store.GetTransaction(orgB.Ctx, txA.ID)
		assert.ErrorIs(t, err, transaction.ErrNotFound)
	})

	t.Run("List", func(t *testing.T) {
		got, err := store.ListTransactions(orgB.Ctx, transaction.ListFilter{})
		require.NoError(t, err)
		assert.Empty(t, got)

		got, err = store.ListTransactions(orgA.Ctx, transaction.ListFilter{})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("Update", func(t *testing.T) {
		stolen := *txA
		stolen.Description = "stolen"
		assert.ErrorIs(t, store.UpdateTransaction(orgB.Ctx, &stolen), transaction.ErrNotFound)

		got, err := store.GetTransaction(orgA.Ctx, txA.ID)
		require.NoError(t, err)
		assert.Equal(t, "PA Gondomar", got.Description)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		err := store.UpdateStatus(orgB.Ctx, txA.ID, transaction.StatusNoInvoice)
		assert.ErrorIs(t, err, transaction.ErrNotFound)
	})

	t.Run("AttachDocument", func(t *testing.T) {
		err := store.AttachDocument(orgB.Ctx, txA.ID, uuid.New())
		assert.ErrorIs(t, err, transaction.ErrNotFound)
	})

	t.Run("DetachDocument", func(t *testing.T) {
		assert.ErrorIs(t, store.DetachDocument(orgB.Ctx, txA.ID), transaction.ErrNotFound)
	})

	t.Run("Delete", func(t *testing.T) {
		assert.ErrorIs(t, store.DeleteTransaction(orgB.Ctx, txA.ID), transaction.ErrNotFound)

		_, err := store.GetTransaction(orgA.Ctx, txA.ID)
		require.NoError(t, err, "org A's transaction must survive org B's delete")
	})

	t.Run("NoAuditRowsLeakedFromRefusedWrites", func(t *testing.T) {
		assert.Zero(t, auditCount(t, orgB.OrgID, audit.SubjectTransaction, txA.ID))
	})
}

func TestTransactionStore_BareContextFailsClosed(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := txstore.New(testDB)
	ctx := context.Background()

	_, err := store.GetTransaction(ctx, uuid.New())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = store.ListTransactions(ctx, transaction.ListFilter{})
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	_, err = store.BeginImport(ctx, time.Now(), time.Now())
	assert.ErrorIs(t, err, org.ErrNoOrgContext)

	assert.ErrorIs(t, store.CreateTransaction(ctx, &transaction.Transaction{}), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.UpdateTransaction(ctx, &transaction.Transaction{}), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.UpdateStatus(ctx, uuid.New(), transaction.StatusComplete), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.DeleteTransaction(ctx, uuid.New()), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.AttachDocument(ctx, uuid.New(), uuid.New()), org.ErrNoOrgContext)
	assert.ErrorIs(t, store.DetachDocument(ctx, uuid.New()), org.ErrNoOrgContext)
}

// A status change must leave exactly one audit row carrying the predecessor it
// actually replaced — the locked read is what makes old_value trustworthy.
func TestTransactionStore_StatusChangeIsAudited(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := txstore.New(testDB)
	tn := newTenant(t, "Status Audit")
	tx := newTransaction(t, store, tn)

	before := auditCount(t, tn.OrgID, audit.SubjectTransaction, tx.ID)
	require.Equal(t, 1, before, "creation is audited")

	require.NoError(t, store.UpdateStatus(tn.Ctx, tx.ID, transaction.StatusNoInvoice))

	assert.Equal(t, 2, auditCount(t, tn.OrgID, audit.SubjectTransaction, tx.ID))

	action, actor, oldValue, newValue := lastAudit(t, tn.OrgID, audit.SubjectTransaction, tx.ID)
	assert.Equal(t, string(audit.ActionStatusChange), action)
	assert.Equal(t, tn.UserID, actor)

	var gotOld, gotNew map[string]any
	require.NoError(t, json.Unmarshal(oldValue, &gotOld))
	require.NoError(t, json.Unmarshal(newValue, &gotNew))

	assert.Equal(t, string(transaction.StatusPendingInvoice), gotOld["status"])
	assert.Equal(t, string(transaction.StatusNoInvoice), gotNew["status"])
}

// Two organizations importing overlapping date ranges must not serialize on
// one advisory lock: the key hashed only the dates before tenancy.
func TestTransactionStore_ImportLockIsOrgScoped(t *testing.T) {
	t.Parallel()
	requireDB(t)

	store := txstore.New(testDB)
	orgA := newTenant(t, "Import Org A")
	orgB := newTenant(t, "Import Org B")

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)

	itxA, err := store.BeginImport(orgA.Ctx, from, to)
	require.NoError(t, err)

	defer itxA.Rollback() //nolint:errcheck

	done := make(chan error, 1)

	go func() {
		itxB, err := store.BeginImport(orgB.Ctx, from, to)
		if err != nil {
			done <- err
			return
		}

		done <- itxB.Rollback()
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("org B blocked on org A's import lock for the same date range")
	}
}
