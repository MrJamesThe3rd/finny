package cgd

import (
	"strings"

	"github.com/shopspring/decimal"
)

// parseEuropeanAmount parses a European-formatted amount string into cents.
// Format examples: "1.234,56" -> 123456, "-588,74" -> -58874, "10,00" -> 1000.
func parseEuropeanAmount(s string) (int64, error) {
	clean := strings.ReplaceAll(s, ".", "")
	clean = strings.ReplaceAll(clean, ",", ".")

	d, err := decimal.NewFromString(clean)
	if err != nil {
		return 0, err
	}

	return d.Mul(decimal.NewFromInt(100)).Round(0).IntPart(), nil
}
