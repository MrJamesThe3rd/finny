package encoding

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

var (
	bomUTF8    = []byte{0xEF, 0xBB, 0xBF}
	bomUTF16LE = []byte{0xFF, 0xFE}
	bomUTF16BE = []byte{0xFE, 0xFF}
)

// NewUTF8Reader detects the encoding of the input and returns a reader
// that decodes the content to UTF-8.
//
// Detection order:
//  1. Check for BOM (UTF-8 BOM is stripped; UTF-16 LE/BE is decoded)
//  2. Validate if the content is valid UTF-8 and return as-is
//  3. Heuristic detection via chardet
//  4. Fallback to Windows-1252
func NewUTF8Reader(r io.Reader) (io.Reader, error) {
	br := bufio.NewReader(r)

	// Peek enough bytes for BOM detection and charset heuristics.
	buf, err := br.Peek(4096)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("peek: %w", err)
	}

	// 1. Check for BOM.
	if bytes.HasPrefix(buf, bomUTF8) {
		// Discard the 3-byte UTF-8 BOM and return the rest as-is.
		_, _ = br.Discard(len(bomUTF8))
		return br, nil
	}

	if bytes.HasPrefix(buf, bomUTF16LE) {
		decoder := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		return transform.NewReader(br, decoder), nil
	}

	if bytes.HasPrefix(buf, bomUTF16BE) {
		decoder := unicode.UTF16(unicode.BigEndian, unicode.UseBOM).NewDecoder()
		return transform.NewReader(br, decoder), nil
	}

	// 2. If the content is valid UTF-8, return as-is.
	if utf8.Valid(buf) {
		return br, nil
	}

	// 3. Heuristic detection via chardet.
	detector := chardet.NewTextDetector()

	result, detectErr := detector.DetectBest(buf)
	if detectErr == nil {
		switch result.Charset {
		case "UTF-8":
			return br, nil
		case "ISO-8859-1", "windows-1252":
			return transform.NewReader(br, charmap.Windows1252.NewDecoder()), nil
		case "ISO-8859-9":
			return transform.NewReader(br, charmap.ISO8859_9.NewDecoder()), nil
		}
	}

	// 4. Fallback to Windows-1252.
	return transform.NewReader(br, charmap.Windows1252.NewDecoder()), nil
}
