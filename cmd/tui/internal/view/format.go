package view

import (
	"context"
	"fmt"
	"time"
)

const dbTimeout = 5 * time.Second

// FormatAmount formats an amount stored as cents into a human-readable string.
func FormatAmount(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100.0)
}

// FormatDate formats a time.Time into YYYY-MM-DD.
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// DbCtx returns a context with a standard timeout for database operations.
func DbCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), dbTimeout)
}
