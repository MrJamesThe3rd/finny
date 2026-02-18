package encoding_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MrJamesThe3rd/finny/internal/encoding"
)

func TestNewUTF8Reader_UTF8Passthrough(t *testing.T) {
	// Valid UTF-8 with Portuguese characters should pass through unchanged.
	input := "Descrição;Montante\nCafé;12,50\nOperação;-3,00\n"
	r, err := encoding.NewUTF8Reader(bytes.NewReader([]byte(input)))
	require.NoError(t, err)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, input, string(got))
}

func TestNewUTF8Reader_Latin1(t *testing.T) {
	// Windows-1252 encoded "Descrição;Montante\n".
	// In Windows-1252: ç = 0xE7, ã = 0xE3
	latin1Bytes := []byte{
		'D', 'e', 's', 'c', 'r', 'i', 0xE7, 0xE3, 'o', ';',
		'M', 'o', 'n', 't', 'a', 'n', 't', 'e', '\n',
	}

	r, err := encoding.NewUTF8Reader(bytes.NewReader(latin1Bytes))
	require.NoError(t, err)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "Descrição;Montante\n", string(got))
}

func TestNewUTF8Reader_UTF8BOM(t *testing.T) {
	// UTF-8 BOM (0xEF 0xBB 0xBF) should be stripped.
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := []byte("Descrição;Montante\n")
	input := append(bom, content...)

	r, err := encoding.NewUTF8Reader(bytes.NewReader(input))
	require.NoError(t, err)

	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "Descrição;Montante\n", string(got))
}
