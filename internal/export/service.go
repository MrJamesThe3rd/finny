package export

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

// Item represents a single exported transaction with its local file path.
type Item struct {
	Transaction *transaction.Transaction
	FilePath    string
}

// Service handles the export of transactions and invoices.
type Service struct {
	transactions *transaction.Service
	client       *http.Client
	apiToken     string
}

// NewService creates a new ExportService.
func NewService(txService *transaction.Service, apiToken string) *Service {
	return &Service{
		transactions: txService,
		client:       &http.Client{Timeout: 30 * time.Second},
		apiToken:     apiToken,
	}
}

// Export downloads invoices for transactions matching the filter to the output directory.
// It returns a list of items linking transactions to their downloaded files.
func (s *Service) Export(ctx context.Context, filter transaction.ListFilter, outputDir string) ([]Item, error) {
	transactions, err := s.transactions.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing transactions: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Pre-allocate to avoid reallocations.
	items := make([]Item, 0, len(transactions))

	for _, t := range transactions {
		item := Item{
			Transaction: t,
		}

		if t.Invoice != nil && t.Invoice.URL != "" {
			path, err := s.downloadInvoice(ctx, t, outputDir)
			if err != nil {
				return nil, fmt.Errorf("downloading invoice for transaction %s: %w", t.ID, err)
			}

			item.FilePath = path
		}

		items = append(items, item)
	}

	return items, nil
}

func (s *Service) downloadInvoice(ctx context.Context, tx *transaction.Transaction, dir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tx.Invoice.URL, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	if s.apiToken != "" {
		req.Header.Set("Authorization", "Token "+s.apiToken)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code %d for url %s", resp.StatusCode, tx.Invoice.URL)
	}

	filename := s.determineFilename(resp, tx)
	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return path, nil
}

func (s *Service) determineFilename(resp *http.Response, tx *transaction.Transaction) string {
	// 1. Try to get filename from Content-Disposition header.
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if filename, ok := params["filename"]; ok && filename != "" {
				// Basic sanitization of the filename from the server
				return strings.ReplaceAll(filepath.Base(filename), " ", "_")
			}
		}
	}

	// 2. Fallback: Generate a name from transaction details.
	ext := ".pdf" // Default assumption

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		if exts, _ := mime.ExtensionsByType(ct); len(exts) > 0 {
			ext = exts[0]
		}
	}

	// Sanitize description for use in filename
	safeDesc := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}

		return '_'
	}, tx.Description)

	// Format: YYYYMMDD_Description.ext
	return fmt.Sprintf("%s_%s%s", tx.Date.Format("20060102"), safeDesc, ext)
}

// GenerateEmailBody creates a formatted email body from the exported items.
func (s *Service) GenerateEmailBody(items []Item) string {
	var sb strings.Builder

	for _, item := range items {
		date := item.Transaction.Date.Format("2006-01-02")
		amount := float64(item.Transaction.Amount) / 100.0
		desc := item.Transaction.Description

		sign := "-"
		if item.Transaction.Type == transaction.TypeIncome {
			sign = "+"
		}

		fileStatus := "Sem Fatura"
		if item.FilePath != "" {
			fileStatus = filepath.Base(item.FilePath)
		}

		sb.WriteString(fmt.Sprintf("* %s | %s | %s%.2f € | %s\n", date, desc, sign, amount, fileStatus))
	}

	return sb.String()
}
