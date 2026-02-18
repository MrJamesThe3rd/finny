package cgd

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"

	enc "github.com/MrJamesThe3rd/finny/internal/encoding"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

// Parser reads CGD bank CSV exports and produces transaction params.
// It auto-detects which CGD format (conta, extrato, cartão) is being used
// by matching column headers against known profiles.
type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(r io.Reader) ([]transaction.CreateParams, error) {
	utf8r, err := enc.NewUTF8Reader(r)
	if err != nil {
		return nil, fmt.Errorf("detect encoding: %w", err)
	}

	reader := csv.NewReader(utf8r)
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}

	profile, colMap, headerIdx := detectProfile(rows)
	if profile == nil {
		return nil, fmt.Errorf("no matching CGD format found: expected columns for conta, extrato, or cartão")
	}

	return parseRows(profile, colMap, rows[headerIdx+1:], headerIdx+1)
}

// colIndex maps column names to their index in the row.
type colIndex map[string]int

// detectProfile scans rows for a header that matches a known profile.
// Returns the matched profile, column index map, and header row index.
func detectProfile(rows [][]string) (*Profile, colIndex, int) {
	for rowIdx, row := range rows {
		cols := make(colIndex)

		for i, cell := range row {
			name := strings.TrimSpace(cell)
			if name != "" {
				cols[name] = i
			}
		}

		for i := range profiles {
			if matchesProfile(&profiles[i], cols) {
				return &profiles[i], cols, rowIdx
			}
		}
	}

	return nil, nil, 0
}

// matchesProfile checks if all required columns of a profile are present.
func matchesProfile(p *Profile, cols colIndex) bool {
	for _, name := range p.requiredCols() {
		if _, ok := cols[name]; !ok {
			return false
		}
	}

	return true
}

// parseRows extracts transactions from data rows using the matched profile.
// headerRowNum is the 0-based index of the header in the original file (for error messages).
func parseRows(p *Profile, cols colIndex, rows [][]string, headerRowNum int) ([]transaction.CreateParams, error) {
	dateIdx := cols[p.DateCol]
	descIdx := cols[p.DescCol]

	var txs []transaction.CreateParams

	for i, row := range rows {
		rowNum := headerRowNum + i + 2 // 1-based, skipping header

		date, ok := parseDate(row, dateIdx)
		if !ok {
			continue
		}

		desc := cellValue(row, descIdx)
		if desc == "" {
			return nil, fmt.Errorf("row %d: missing description", rowNum)
		}

		amount, txType, ok := parseAmount(p, cols, row)
		if !ok {
			continue
		}

		txs = append(txs, transaction.CreateParams{
			Amount:         amount,
			Type:           txType,
			Status:         transaction.StatusDraft,
			Description:    desc,
			RawDescription: desc,
			Date:           date,
		})
	}

	return txs, nil
}

// parseDate tries to parse a date from the given cell index.
// Returns false for empty cells or unparseable values (footer rows, etc).
func parseDate(row []string, idx int) (time.Time, bool) {
	s := cellValue(row, idx)
	if s == "" {
		return time.Time{}, false
	}

	t, err := time.Parse("02-01-2006", s)
	if err != nil {
		return time.Time{}, false
	}

	return t, true
}

// parseAmount extracts the amount and transaction type from a row based on the profile's amount mode.
func parseAmount(p *Profile, cols colIndex, row []string) (int64, transaction.Type, bool) {
	switch p.AmountMode {
	case amountSingle:
		return parseSingleAmount(row, cols[p.AmountCol])
	case amountSplit:
		return parseSplitAmount(row, cols[p.DebitCol], cols[p.CreditCol])
	}

	return 0, "", false
}

// parseSingleAmount handles a single signed amount column.
func parseSingleAmount(row []string, idx int) (int64, transaction.Type, bool) {
	s := cellValue(row, idx)
	if s == "" {
		return 0, "", false
	}

	cents, err := parseEuropeanAmount(s)
	if err != nil {
		return 0, "", false
	}

	if cents == 0 {
		return 0, "", false
	}

	if cents < 0 {
		return -cents, transaction.TypeExpense, true
	}

	return cents, transaction.TypeIncome, true
}

// parseSplitAmount handles separate debit/credit columns.
func parseSplitAmount(row []string, debitIdx, creditIdx int) (int64, transaction.Type, bool) {
	if s := cellValue(row, debitIdx); s != "" {
		cents, err := parseEuropeanAmount(s)
		if err == nil && cents != 0 {
			return abs(cents), transaction.TypeExpense, true
		}
	}

	if s := cellValue(row, creditIdx); s != "" {
		cents, err := parseEuropeanAmount(s)
		if err == nil && cents != 0 {
			return abs(cents), transaction.TypeIncome, true
		}
	}

	return 0, "", false
}

// cellValue safely gets a trimmed cell value from a row.
func cellValue(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}

	return strings.TrimSpace(row[idx])
}

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}

	return n
}
