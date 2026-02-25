package tui

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/hex"
	"io"
	"strings"
	"unicode/utf8"
)

// TryDecodeUserdata attempts to decode userdata that may be base64, base64+gzip, or hex encoded.
// Returns (decoded string, true) on success, or ("", false) on failure or unknown encoding.
// Failures gracefully fall through so the caller can display the raw value.
func TryDecodeUserdata(s string) (result string, ok bool) {
	defer func() {
		if recover() != nil {
			result = ""
			ok = false
		}
	}()

	if s == "" || s == "null" || strings.HasPrefix(s, "(") {
		return "", false
	}

	// Try base64 variants first (with whitespace stripping)
	stripped := stripBase64Whitespace(s)
	if decoded, ok := tryBase64Variants(stripped); ok {
		if validated, ok := validateDecoded(decoded); ok {
			return validated, true
		}
	}

	// Fall back to hex
	if decoded, ok := tryHexDecode(s); ok {
		if validated, ok := validateDecoded(decoded); ok {
			return validated, true
		}
	}

	return "", false
}

func stripBase64Whitespace(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

func tryBase64Variants(s string) ([]byte, bool) {
	encodings := []*base64.Encoding{
		base64.StdEncoding,
		base64.URLEncoding,
		base64.RawStdEncoding,
		base64.RawURLEncoding,
	}

	for _, enc := range encodings {
		decoded, err := enc.DecodeString(s)
		if err == nil {
			// Try gzip decompression if it looks like gzip
			if decompressed, ok := tryGzipDecompress(decoded); ok {
				return decompressed, true
			}
			return decoded, true
		}
	}

	return nil, false
}

func tryGzipDecompress(b []byte) ([]byte, bool) {
	if len(b) < 2 || b[0] != 0x1f || b[1] != 0x8b {
		return nil, false
	}

	r, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, false
	}
	defer r.Close()

	decompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, false
	}

	return decompressed, true
}

func tryHexDecode(s string) ([]byte, bool) {
	if len(s)%2 != 0 {
		return nil, false
	}
	for _, c := range s {
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			continue
		}
		return nil, false
	}

	decoded, err := hex.DecodeString(s)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func validateDecoded(b []byte) (string, bool) {
	for _, c := range b {
		if c == 0 {
			return "", false
		}
	}

	if !utf8.Valid(b) {
		return "", false
	}

	return string(b), true
}
