package view

import (
	"context"
	"fmt"
	"time"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

const dbTimeout = 5 * time.Second

// FormatAmountSigned formats an amount with a + or - sign based on transaction type.
func FormatAmountSigned(cents int64, txType transaction.Type) string {
	if txType == transaction.TypeIncome {
		return fmt.Sprintf("+%.2f", float64(cents)/100.0)
	}
	return fmt.Sprintf("-%.2f", float64(cents)/100.0)
}

// FormatDate formats a time.Time into YYYY-MM-DD.
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// DbCtx returns a child of parent with a standard timeout for database operations.
func DbCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, dbTimeout)
}
