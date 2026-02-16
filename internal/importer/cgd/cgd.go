package cgd

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

const (
	colDate   = "Data mov."
	colDesc   = "Descrição"
	colAmount = "Montante"
)

type Importer struct{}

func New() *Importer {
	return &Importer{}
}

func (i *Importer) Parse(r io.Reader) ([]transaction.CreateParams, error) {
	reader := csv.NewReader(r)
	reader.Comma = ';'
	reader.FieldsPerRecord = -1 // Allow variable number of fields
	reader.LazyQuotes = true    // Allow sloppy quotes if necessary

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read csv: %w", err)
	}

	var txs []transaction.CreateParams

	headerFound := false

	// Column indices
	idxDate := -1
	idxDesc := -1
	idxAmount := -1

	for _, row := range rows {
		// 1. Search for header landmark
		if !headerFound {
			// Check if this row looks like the header
			// We map indices if we find the columns
			matches := 0

			for i, col := range row {
				col = strings.TrimSpace(col)
				switch col {
				case colDate:
					idxDate = i
					matches++
				case colDesc:
					idxDesc = i
					matches++
				case colAmount:
					idxAmount = i
					matches++
				}
			}

			// If we found at least Date and Amount, we assume it's the header
			if matches >= 2 {
				headerFound = true
			}

			continue
		}

		// 2. Parse Data Rows
		// Ensure we have enough columns
		maxIdx := max(idxDate, max(idxDesc, idxAmount))
		if len(row) <= maxIdx {
			continue
		}

		// Parse Date
		dateStr := strings.TrimSpace(row[idxDate])
		if dateStr == "" {
			continue
		}

		date, err := time.Parse("02-01-2006", dateStr)
		if err != nil {
			// Probably not a date row (maybe footer)
			continue
		}

		// Parse Description
		description := ""
		if idxDesc != -1 {
			description = strings.TrimSpace(row[idxDesc])
		}

		// Parse Amount
		amountStr := strings.TrimSpace(row[idxAmount])

		amount, err := parseAmount(amountStr)
		if err != nil {
			// Skip valid dates with invalid amounts? or error?
			// Let's skip for robustness
			continue
		}

		// Determine Type
		txType := transaction.TypeIncome
		if amount < 0 {
			txType = transaction.TypeExpense
			amount = -amount // Store absolute value
		}

		txs = append(txs, transaction.CreateParams{
			Amount:         amount,
			Type:           txType,
			Status:         transaction.StatusDraft,
			Description:    description,
			RawDescription: description,
			Date:           date,
		})
	}

	return txs, nil
}

func parseAmount(s string) (int64, error) {
	// Format: "1.234,56" or "-588,74"
	// Remove dots (thousand separators)
	clean := strings.ReplaceAll(s, ".", "")
	// Replace comma with dot (decimal separator)
	clean = strings.ReplaceAll(clean, ",", ".")

	val, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, err
	}

	// Convert to cents
	return int64(math.Round(val * 100)), nil
}
