package cgd_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/MrJamesThe3rd/finny/internal/importer/cgd"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

func TestImporter_Parse(t *testing.T) {
	type args struct {
		csvContent string
	}

	type testCase struct {
		name    string
		args    args
		wantLen int
		verify  func(t *testing.T, txs []transaction.CreateParams)
		wantErr bool
	}

	tests := []testCase{
		{
			name: "Standard CGD Export",
			args: args{
				csvContent: `Consultar saldos e movimentos à ordem - 31-01-2026;"=""0000"""
Nome cliente;JOHN DOE
NIF;"=""123"""

 Dados da conta
Conta;0000 - EUR - Conta Extracto
Saldo contabilístico;1.000,00 EUR
Saldo disponível;1.000,00 EUR

Dados da consulta
Período;Últimos 90 dias
Intervalo de;01-01-2026 a 31-01-2026
Tipos de movimento;Todos

Data mov.;Data-valor;Descrição;Montante;Saldo contabilístico após movimento
30-01-2026;30-01-2026;TEST_EXPENSE;-10,00;990,00
09-01-2026;09-01-2026;TEST_INCOME;50,00;1.040,00
`,
			},
			wantLen: 2,
			verify: func(t *testing.T, txs []transaction.CreateParams) {
				// Assertions for first transaction (Expense)
				assert.Equal(t, "TEST_EXPENSE", txs[0].Description)
				assert.Equal(t, int64(1000), txs[0].Amount)
				assert.Equal(t, transaction.TypeExpense, txs[0].Type)

				expectedDate, _ := time.Parse("02-01-2006", "30-01-2026")
				assert.True(t, txs[0].Date.Equal(expectedDate))

				// Assertions for second transaction (Income)
				assert.Equal(t, "TEST_INCOME", txs[1].Description)
				assert.Equal(t, int64(5000), txs[1].Amount)
				assert.Equal(t, transaction.TypeIncome, txs[1].Type)
			},
			wantErr: false,
		},
		{
			name: "Empty File",
			args: args{
				csvContent: "",
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "Header Only",
			args: args{
				csvContent: `Data mov.;Data-valor;Descrição;Montante`,
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name: "Different Column Order",
			args: args{
				csvContent: `Random;MetaData
Montante;Descrição;Data mov.;Ignored
-10,00;TEST_ORDER;30-01-2026;XXX
`,
			},
			wantLen: 1,
			verify: func(t *testing.T, txs []transaction.CreateParams) {
				assert.Equal(t, "TEST_ORDER", txs[0].Description)
				assert.Equal(t, int64(1000), txs[0].Amount)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			importer := cgd.New()
			r := strings.NewReader(tt.args.csvContent)
			got, err := importer.Parse(r)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Len(t, got, tt.wantLen)

			if tt.verify != nil {
				tt.verify(t, got)
			}
		})
	}
}
