package export

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/document"
	docstore "github.com/MrJamesThe3rd/finny/internal/document/store"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

// ── transaction repository stub ───────────────────────────────────────────────

type mockTxRepo struct {
	txs []*transaction.Transaction
}

func (m *mockTxRepo) CreateTransaction(_ context.Context, _ *transaction.Transaction) error {
	return nil
}
func (m *mockTxRepo) GetTransaction(_ context.Context, _ uuid.UUID) (*transaction.Transaction, error) {
	return nil, nil
}
func (m *mockTxRepo) UpdateTransaction(_ context.Context, _ *transaction.Transaction) error {
	return nil
}
func (m *mockTxRepo) ListTransactions(_ context.Context, _ transaction.ListFilter) ([]*transaction.Transaction, error) {
	return m.txs, nil
}
func (m *mockTxRepo) DeleteTransaction(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockTxRepo) AttachDocument(_ context.Context, _ uuid.UUID, _ uuid.UUID) error {
	return nil
}
func (m *mockTxRepo) DetachDocument(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockTxRepo) UpdateStatus(_ context.Context, _ uuid.UUID, _ transaction.Status) error {
	return nil
}
func (m *mockTxRepo) BeginImport(_ context.Context, _, _ time.Time) (transaction.ImportTx, error) {
	return nil, nil
}

// ── document repository stub ──────────────────────────────────────────────────

type mockDocRepo struct {
	docs      map[uuid.UUID]*document.Document
	locations map[uuid.UUID][]document.Location
	backends  map[uuid.UUID]*document.BackendConfig
}

func (m *mockDocRepo) ListBackends(_ context.Context) ([]document.BackendConfig, error) {
	var out []document.BackendConfig
	for _, b := range m.backends {
		out = append(out, *b)
	}
	return out, nil
}
func (m *mockDocRepo) GetBackend(_ context.Context, id uuid.UUID) (*document.BackendConfig, error) {
	b, ok := m.backends[id]
	if !ok {
		return nil, fmt.Errorf("backend not found: %s", id)
	}
	return b, nil
}
func (m *mockDocRepo) SetBackendConfig(_ context.Context, _ uuid.UUID, _ json.RawMessage) error {
	return nil
}
func (m *mockDocRepo) CreateBackend(_ context.Context, cfg *document.BackendConfig) error {
	cfg.ID = uuid.New()
	m.backends[cfg.ID] = cfg
	return nil
}
func (m *mockDocRepo) UpdateBackend(_ context.Context, _ uuid.UUID, _ *string, _ json.RawMessage, _ *bool) error {
	return nil
}
func (m *mockDocRepo) DeleteBackend(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockDocRepo) BackendHasDocuments(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}
func (m *mockDocRepo) DeleteDocument(_ context.Context, id uuid.UUID) error {
	delete(m.docs, id)
	return nil
}
func (m *mockDocRepo) CreateDocument(_ context.Context, doc *document.Document) error {
	doc.ID = uuid.New()
	m.docs[doc.ID] = doc
	return nil
}
func (m *mockDocRepo) GetDocument(_ context.Context, id uuid.UUID) (*document.Document, error) {
	d, ok := m.docs[id]
	if !ok {
		return nil, document.ErrDocumentNotFound
	}
	return d, nil
}
func (m *mockDocRepo) AddLocation(_ context.Context, loc *document.Location) error {
	m.locations[loc.DocumentID] = append(m.locations[loc.DocumentID], *loc)
	return nil
}
func (m *mockDocRepo) ListLocations(_ context.Context, documentID uuid.UUID) ([]document.Location, error) {
	return m.locations[documentID], nil
}

// ── fake backend ──────────────────────────────────────────────────────────────

type fakeBackend struct {
	content string
}

func (f *fakeBackend) Type() string { return "fake" }
func (f *fakeBackend) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.content)), nil
}
func (f *fakeBackend) Upload(_ context.Context, _ string, _ io.Reader) (string, error) {
	return "", nil
}
func (f *fakeBackend) Delete(_ context.Context, _ string) error { return nil }

// ── helpers ───────────────────────────────────────────────────────────────────

func buildDocRepo() (*mockDocRepo, uuid.UUID, uuid.UUID) {
	backendID := uuid.New()
	docID := uuid.New()

	doc := &document.Document{
		ID:       docID,
		Filename: "invoice.pdf",
		MIMEType: "application/pdf",
	}

	backend := &document.BackendConfig{
		ID:      backendID,
		Type:    "fake",
		Enabled: true,
		Config:  json.RawMessage(`{}`),
	}

	repo := &mockDocRepo{
		docs:     map[uuid.UUID]*document.Document{docID: doc},
		backends: map[uuid.UUID]*document.BackendConfig{backendID: backend},
		locations: map[uuid.UUID][]document.Location{
			docID: {{DocumentID: docID, BackendID: backendID, Key: "42"}},
		},
	}

	return repo, docID, backendID
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestExportService_Export(t *testing.T) {
	tmpDir := t.TempDir()
	date := time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC)

	docRepo, docID, _ := buildDocRepo()

	docID2 := uuid.New()
	doc2 := &document.Document{ID: docID2, Filename: "", MIMEType: "application/pdf"}
	docRepo.docs[docID2] = doc2
	backendID2 := uuid.New()
	docRepo.backends[backendID2] = &document.BackendConfig{
		ID: backendID2, Type: "fake", Enabled: true, Config: json.RawMessage(`{}`),
	}
	docRepo.locations[docID2] = []document.Location{{DocumentID: docID2, BackendID: backendID2, Key: "99"}}

	registry := document.NewRegistry()
	registry.Register("fake", func(_ json.RawMessage) (document.Backend, error) {
		return &fakeBackend{content: "fake pdf content"}, nil
	})

	txs := []*transaction.Transaction{
		{ID: uuid.New(), Amount: 1000, Description: "Test Transaction 1", Date: date, DocumentID: &docID},
		{ID: uuid.New(), Amount: 2000, Description: "Test Transaction 2", Date: date, DocumentID: &docID2},
		{ID: uuid.New(), Amount: 3000, Description: "No Document", Date: date},
	}

	txSvc := transaction.NewService(&mockTxRepo{txs: txs})
	docSvc := document.NewService(docRepo, registry)

	// Use the docstore package to satisfy the unused-import check in test builds.
	_ = docstore.New

	svc := NewService(txSvc, docSvc)

	items, err := svc.Export(context.Background(), transaction.ListFilter{}, tmpDir)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Item 1 — named document
	if items[0].FilePath == "" {
		t.Error("item 1: expected non-empty FilePath")
	}

	// Item 2 — unnamed document; filename generated from transaction
	if items[1].FilePath == "" {
		t.Error("item 2: expected non-empty FilePath")
	}

	// Item 3 — no document
	if items[2].FilePath != "" {
		t.Errorf("item 3: expected empty FilePath, got %s", items[2].FilePath)
	}
}

func TestService_GenerateSummary(t *testing.T) {
	s := &Service{}

	date := time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC)
	items := []Item{
		{
			Transaction: &transaction.Transaction{
				Date:        date,
				Amount:      1250,
				Description: "Hosting",
				Type:        transaction.TypeExpense,
			},
			FilePath: "/tmp/invoice.pdf",
		},
		{
			Transaction: &transaction.Transaction{
				Date:        date,
				Amount:      500,
				Description: "Coffee",
				Type:        transaction.TypeExpense,
			},
			FilePath: "",
		},
	}

	body := s.GenerateSummary(items)

	for _, want := range []string{
		"2023-10-27 | Hosting | -12.50 € | invoice.pdf",
		"2023-10-27 | Coffee | -5.00 € | Sem Fatura",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected summary to contain %q\ngot:\n%s", want, body)
		}
	}
}
