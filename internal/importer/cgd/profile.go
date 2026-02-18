package cgd

// amountMode determines how amounts are extracted from a row.
type amountMode int

const (
	// amountSingle means one signed column (e.g. "Montante" with value "-10,00").
	amountSingle amountMode = iota
	// amountSplit means separate debit and credit columns (e.g. "Débito"/"Crédito").
	amountSplit
)

// Profile describes the column layout of a CGD CSV export format.
// Adding a new format is just adding a new Profile to the profiles slice.
type Profile struct {
	Name       string
	DateCol    string
	DescCol    string
	AmountMode amountMode
	AmountCol  string // used when AmountMode == amountSingle
	DebitCol   string // used when AmountMode == amountSplit
	CreditCol  string // used when AmountMode == amountSplit
}

// requiredCols returns the column names that must be present for this profile to match.
func (p Profile) requiredCols() []string {
	cols := []string{p.DateCol, p.DescCol}

	switch p.AmountMode {
	case amountSingle:
		cols = append(cols, p.AmountCol)
	case amountSplit:
		cols = append(cols, p.DebitCol, p.CreditCol)
	}

	return cols
}

// profiles is the ordered list of CGD export formats to try during auto-detection.
// More specific profiles should come first to avoid false matches.
var profiles = []Profile{
	{
		Name:       "cartão",
		DateCol:    "Data",
		DescCol:    "Descrição",
		AmountMode: amountSplit,
		DebitCol:   "Débito",
		CreditCol:  "Crédito",
	},
	{
		Name:       "extrato",
		DateCol:    "Data mov.",
		DescCol:    "Descrição",
		AmountMode: amountSingle,
		AmountCol:  "Movimento",
	},
	{
		Name:       "conta",
		DateCol:    "Data mov.",
		DescCol:    "Descrição",
		AmountMode: amountSingle,
		AmountCol:  "Montante",
	},
}
