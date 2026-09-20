package transaction_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

func TestOwnerOnlyStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		give transaction.Status
		want bool
	}{
		{
			name: "NoInvoiceIsOwnerOnly",
			give: transaction.StatusNoInvoice,
			want: true,
		},
		{
			name: "DraftIsOwnerOnly",
			give: transaction.StatusDraft,
			want: true,
		},
		{
			name: "AccountantMayFlagBackToPendingInvoice",
			give: transaction.StatusPendingInvoice,
			want: false,
		},
		{
			name: "AccountantMayComplete",
			give: transaction.StatusComplete,
			want: false,
		},
		{
			name: "UnknownStatusIsNotOwnerOnly",
			give: transaction.Status("bogus"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, transaction.OwnerOnlyStatus(tt.give))
		})
	}
}
