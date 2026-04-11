package export

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/MrJamesThe3rd/finny/internal/document"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

// Item represents a single exported transaction with its local file path.
type Item struct {
	Transaction *transaction.Transaction
	FilePath    string
}

// Service handles the export of transactions and their associated documents.
type Service struct {
	transactions *transaction.Service
	docs         *document.Service
}

// NewService creates a new export Service.
func NewService(txService *transaction.Service, docService *document.Service) *Service {
	return &Service{
		transactions: txService,
		docs:         docService,
	}
}

// Export downloads documents for transactions matching the filter to the output directory.
// It returns a list of items linking transactions to their downloaded files.
func (s *Service) Export(ctx context.Context, filter transaction.ListFilter, outputDir string) ([]Item, error) {
	transactions, err := s.transactions.List(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing transactions: %w", err)
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	items := make([]Item, 0, len(transactions))

	for _, t := range transactions {
		item := Item{Transaction: t}

		if t.DocumentID != nil {
			path, err := s.downloadDocument(ctx, t, outputDir)
			if err != nil {
				return nil, fmt.Errorf("downloading document for transaction %s: %w", t.ID, err)
			}

			item.FilePath = path
		}

		items = append(items, item)
	}

	return items, nil
}

func (s *Service) downloadDocument(ctx context.Context, tx *transaction.Transaction, dir string) (string, error) {
	rc, doc, err := s.docs.Download(ctx, *tx.DocumentID)
	if err != nil {
		return "", fmt.Errorf("downloading document: %w", err)
	}
	defer rc.Close()

	filename := doc.Filename
	if filename == "" || filename == "invoice" || filename == "unknown" {
		filename = generateFilename(tx)
	}

	path := filepath.Join(dir, filename)

	f, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("creating file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, rc); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return path, nil
}

// generateFilename produces a deterministic filename from transaction metadata.
func generateFilename(tx *transaction.Transaction) string {
	safeDesc := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}

		return '_'
	}, tx.Description)

	return fmt.Sprintf("%s_%s.pdf", tx.Date.Format("20060102"), safeDesc)
}

// GenerateSummary creates a formatted summary of the exported items.
func (s *Service) GenerateSummary(items []Item) string {
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
