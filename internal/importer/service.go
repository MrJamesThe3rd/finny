package importer

import (
	"fmt"
	"io"

	"github.com/MrJamesThe3rd/finny/internal/importer/cgd"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

type Service struct {
	cgdImporter Importer
}

func NewService() *Service {
	return &Service{
		cgdImporter: cgd.New(),
	}
}

func (s *Service) Import(bank Bank, r io.Reader) ([]transaction.CreateParams, error) {
	var importer Importer

	switch bank {
	case BankCGD:
		importer = s.cgdImporter
	default:
		return nil, fmt.Errorf("unknown bank: %s", bank)
	}

	return importer.Parse(r)
}
