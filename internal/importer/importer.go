package importer

import (
	"io"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type Bank string

const (
	BankCGD Bank = "cgd"
)

type Importer interface {
	Parse(r io.Reader) ([]transaction.CreateParams, error)
}
