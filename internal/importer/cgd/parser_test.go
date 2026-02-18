package cgd_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"

	"github.com/MrJamesThe3rd/finny/internal/importer/cgd"
	"github.com/MrJamesThe3rd/finny/internal/transaction"
)

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func TestParser_Conta(t *testing.T) {
	csv := `Consultar saldos e movimentos à ordem - 31-01-2026;"=""0000"""
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
30-01-2026;30-01-2026;INSTITUTO GESTAO FINA;-588,74;48.825,46
09-01-2026;09-01-2026;TFI Wise;8.608,52;52.532,78
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 2)

	assert.Equal(t, date(2026, 1, 30), txs[0].Date)
	assert.Equal(t, "INSTITUTO GESTAO FINA", txs[0].Description)
	assert.Equal(t, int64(58874), txs[0].Amount)
	assert.Equal(t, transaction.TypeExpense, txs[0].Type)

	assert.Equal(t, date(2026, 1, 9), txs[1].Date)
	assert.Equal(t, "TFI Wise", txs[1].Description)
	assert.Equal(t, int64(860852), txs[1].Amount)
	assert.Equal(t, transaction.TypeIncome, txs[1].Type)
}

func TestParser_Extrato(t *testing.T) {
	csv := `Consultar extrato - 15-02-2026 : 0829015676030
Nome empresa ;VIBRANTGARDEN UNIPESSOAL,LDA
NIF ;517948974
Conta ;0829015676030 - EUR - Conta Extracto
Intervalo de ;01-02-2026 a 14-02-2026
Tipos de movimento ;Todos
Saldo contabilístico Inicial ;48.825,46
Saldo contabilístico final ;41.393,66

Data mov. ;Data valor ;Origem ;Descrição ;Movimento ;Estorno ;Saldo contabilístico após movimento ;
13-02-2026;13-02-2026;"=""0003""";PAGAMENTO TSU ;-608,13;  ;41.393,66;
04-02-2026;04-02-2026;SIBS ;TFI Wise ;4.324,06;  ;51.302,85;
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 2)

	assert.Equal(t, date(2026, 2, 13), txs[0].Date)
	assert.Equal(t, "PAGAMENTO TSU", txs[0].Description)
	assert.Equal(t, int64(60813), txs[0].Amount)
	assert.Equal(t, transaction.TypeExpense, txs[0].Type)

	assert.Equal(t, date(2026, 2, 4), txs[1].Date)
	assert.Equal(t, "TFI Wise", txs[1].Description)
	assert.Equal(t, int64(432406), txs[1].Amount)
	assert.Equal(t, transaction.TypeIncome, txs[1].Type)
}

func TestParser_Cartao(t *testing.T) {
	csv := `Consultar saldos e movimentos de cartões - 15-02-2026
Nome empresa ;VIBRANTGARDEN UNIPESSOAL,LDA
NIF ;517948974

Conta cartão ;4163 **** **** 8016 - EUR - Business Débito
Tipo de movimentos ;Conta à ordem
Desde ;15/12/2025

Data ;Data valor ;Descrição ;Débito ;Crédito ;
16-12-2025 ;14-12-2025 ;PA GONDOMAR         GONDOMAR ;64,00 ; ;
31-12-2025 ;29-12-2025 ;UBER   *TRIP             HELP.UBER.COMNL ;47,91 ; ;
 ; ; ; ;Página 1/2 ;
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 2)

	assert.Equal(t, date(2025, 12, 16), txs[0].Date)
	assert.Equal(t, "PA GONDOMAR         GONDOMAR", txs[0].Description)
	assert.Equal(t, int64(6400), txs[0].Amount)
	assert.Equal(t, transaction.TypeExpense, txs[0].Type)

	assert.Equal(t, date(2025, 12, 31), txs[1].Date)
	assert.Equal(t, "UBER   *TRIP             HELP.UBER.COMNL", txs[1].Description)
	assert.Equal(t, int64(4791), txs[1].Amount)
	assert.Equal(t, transaction.TypeExpense, txs[1].Type)
}

func TestParser_CartaoCredit(t *testing.T) {
	csv := `Data ;Data valor ;Descrição ;Débito ;Crédito ;
16-12-2025 ;14-12-2025 ;REFUND AMAZON ;  ;25,00 ;
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 1)

	assert.Equal(t, int64(2500), txs[0].Amount)
	assert.Equal(t, transaction.TypeIncome, txs[0].Type)
}

func TestParser_Latin1Encoding(t *testing.T) {
	utf8CSV := "Data mov.;Descrição;Montante\n30-01-2026;CAFÉ CENTRAL;-10,00\n"

	encoder := charmap.Windows1252.NewEncoder()
	latin1Bytes, err := encoder.Bytes([]byte(utf8CSV))
	require.NoError(t, err)

	p := cgd.NewParser()
	txs, err := p.Parse(bytes.NewReader(latin1Bytes))
	require.NoError(t, err)
	require.Len(t, txs, 1)

	assert.Equal(t, "CAFÉ CENTRAL", txs[0].RawDescription)
}

func TestParser_DifferentColumnOrder(t *testing.T) {
	csv := `Random;MetaData
Montante;Descrição;Data mov.;Ignored
-10,00;TEST_ORDER;30-01-2026;XXX
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 1)

	assert.Equal(t, "TEST_ORDER", txs[0].Description)
	assert.Equal(t, int64(1000), txs[0].Amount)
}

func TestParser_EmptyFile(t *testing.T) {
	p := cgd.NewParser()
	_, err := p.Parse(strings.NewReader(""))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no matching CGD format")
}

func TestParser_HeaderOnly(t *testing.T) {
	csv := `Data mov.;Data-valor;Descrição;Montante`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	assert.Empty(t, txs)
}

func TestParser_MissingDescription(t *testing.T) {
	csv := `Data mov.;Descrição;Montante
30-01-2026;;-10,00
`

	p := cgd.NewParser()
	_, err := p.Parse(strings.NewReader(csv))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "description")
}

func TestParser_AllFieldsPopulated(t *testing.T) {
	csv := `Data mov.;Descrição;Montante
30-01-2026;TEST;-10,00
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 1)

	assert.Equal(t, transaction.StatusDraft, txs[0].Status)
	assert.Equal(t, "TEST", txs[0].RawDescription)
	assert.Equal(t, txs[0].Description, txs[0].RawDescription)
}

func TestParser_LargeAmounts(t *testing.T) {
	csv := `Data mov.;Descrição;Montante
30-01-2026;BIG TRANSFER;-1.234.567,89
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 1)

	assert.Equal(t, int64(123456789), txs[0].Amount)
}

func TestParser_SkipsFooterRows(t *testing.T) {
	csv := `Data mov.;Descrição;Montante
30-01-2026;TEST;-10,00
Totais;;;;
`

	p := cgd.NewParser()
	txs, err := p.Parse(strings.NewReader(csv))
	require.NoError(t, err)
	require.Len(t, txs, 1)
}
