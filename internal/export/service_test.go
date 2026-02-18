package export

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

// Mock Repository
type mockRepo struct {
	listTransactionsFunc func(ctx context.Context, filter transaction.ListFilter) ([]*transaction.Transaction, error)
}

func (m *mockRepo) CreateTransaction(ctx context.Context, tx *transaction.Transaction) error {
	return nil
}

func (m *mockRepo) GetTransaction(ctx context.Context, id uuid.UUID) (*transaction.Transaction, error) {
	return nil, nil
}

func (m *mockRepo) UpdateTransaction(ctx context.Context, tx *transaction.Transaction) error {
	return nil
}

func (m *mockRepo) ListTransactions(ctx context.Context, filter transaction.ListFilter) ([]*transaction.Transaction, error) {
	if m.listTransactionsFunc != nil {
		return m.listTransactionsFunc(ctx, filter)
	}

	return nil, nil
}
func (m *mockRepo) DeleteTransaction(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockRepo) UpdateInvoice(ctx context.Context, id uuid.UUID, invoiceURL string) error {
	return nil
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status transaction.Status) error {
	return nil
}

func (m *mockRepo) BeginImport(ctx context.Context, minDate, maxDate time.Time) (transaction.ImportTx, error) {
	return nil, nil
}

func TestExportService_Export(t *testing.T) {
	// Setup HTTP server for invoices
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/invoice.pdf" {
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", "attachment; filename=\"invoice_123.pdf\"")
			w.Write([]byte("fake pdf content"))

			return
		}

		if r.URL.Path == "/invoice_no_filename" {
			w.Header().Set("Content-Type", "application/pdf")
			w.Write([]byte("fake pdf content"))

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Setup Temp Dir
	tmpDir, err := os.MkdirTemp("", "export_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup Data
	id1 := uuid.New()
	id2 := uuid.New()
	date := time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC)

	tx1 := &transaction.Transaction{
		ID:          id1,
		Amount:      1000,
		Description: "Test Transaction 1",
		Date:        date,
		Invoice: &transaction.Invoice{
			ID:  uuid.New(),
			URL: ts.URL + "/invoice.pdf",
		},
	}

	tx2 := &transaction.Transaction{
		ID:          id2,
		Amount:      2000,
		Description: "Test Transaction 2",
		Date:        date,
		Invoice: &transaction.Invoice{
			ID:  uuid.New(),
			URL: ts.URL + "/invoice_no_filename",
		},
	}

	tx3 := &transaction.Transaction{
		ID:          uuid.New(),
		Amount:      3000,
		Description: "No Invoice",
		Date:        date,
	}

	repo := &mockRepo{
		listTransactionsFunc: func(ctx context.Context, filter transaction.ListFilter) ([]*transaction.Transaction, error) {
			return []*transaction.Transaction{tx1, tx2, tx3}, nil
		},
	}

	txService := transaction.NewService(repo)
	service := NewService(txService, "test-token")

	// Execution
	filter := transaction.ListFilter{}

	items, err := service.Export(context.Background(), filter, tmpDir)
	if err != nil {
		t.Fatalf("Export failed: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// Check Item 1 (With filename from header)
	if items[0].Transaction != tx1 {
		t.Errorf("expected item 1 to be tx1")
	}

	if filepath.Base(items[0].FilePath) != "invoice_123.pdf" {
		t.Errorf("expected invoice_123.pdf, got %s", filepath.Base(items[0].FilePath))
	}

	content1, _ := os.ReadFile(items[0].FilePath)
	if string(content1) != "fake pdf content" {
		t.Errorf("file content mismatch")
	}

	// Check Item 2 (Generated filename)
	if items[1].Transaction != tx2 {
		t.Errorf("expected item 2 to be tx2")
	}

	expectedName2 := "20231027_Test_Transaction_2.pdf"
	if filepath.Base(items[1].FilePath) != expectedName2 {
		t.Errorf("expected %s, got %s", expectedName2, filepath.Base(items[1].FilePath))
	}

	// Check Item 3 (No invoice)
	if items[2].Transaction != tx3 {
		t.Errorf("expected item 3 to be tx3")
	}

	if items[2].FilePath != "" {
		t.Errorf("expected empty file path for item 3, got %s", items[2].FilePath)
	}
}

func TestService_GenerateSummary(t *testing.T) {
	s := &Service{}

	date := time.Date(2023, 10, 27, 0, 0, 0, 0, time.UTC)
	items := []Item{
		{
			Transaction: &transaction.Transaction{
				Date:        date,
				Amount:      1250, // 12.50
				Description: "Hosting",
			},
			FilePath: "/tmp/invoice.pdf",
		},
		{
			Transaction: &transaction.Transaction{
				Date:        date,
				Amount:      500, // 5.00
				Description: "Coffee",
			},
			FilePath: "",
		},
	}

	body := s.GenerateSummary(items)

	expectedSubstrings := []string{
		"2023-10-27 | Hosting | -12.50 € | invoice.pdf",
		"2023-10-27 | Coffee | -5.00 € | Sem Fatura",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(body, sub) {
			t.Errorf("expected body to contain %q", sub)
		}
	}
}
