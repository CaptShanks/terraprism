package tui

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"
)

func TestTryDecodeUserdata_StdBase64(t *testing.T) {
	decoded, ok := TryDecodeUserdata("aGVsbG8=")
	if !ok {
		t.Fatal("expected decode success")
	}
	if decoded != "hello" {
		t.Errorf("got %q, want %q", decoded, "hello")
	}
}

func TestTryDecodeUserdata_Base64WithNewlines(t *testing.T) {
	decoded, ok := TryDecodeUserdata("aGVs\nbG8=")
	if !ok {
		t.Fatal("expected decode success")
	}
	if decoded != "hello" {
		t.Errorf("got %q, want %q", decoded, "hello")
	}
}

func TestTryDecodeUserdata_URLBase64(t *testing.T) {
	// ">>>" in URL-safe base64: StdEncoding gives "Pj4+", URLEncoding gives "Pj4-" (- replaces +)
	decoded, ok := TryDecodeUserdata("Pj4-")
	if !ok {
		t.Fatal("expected decode success")
	}
	if decoded != ">>>" {
		t.Errorf("got %q, want %q", decoded, ">>>")
	}
}

func TestTryDecodeUserdata_RawURLBase64(t *testing.T) {
	// "hello" without padding = "aGVsbG8"
	decoded, ok := TryDecodeUserdata("aGVsbG8")
	if !ok {
		t.Fatal("expected decode success")
	}
	if decoded != "hello" {
		t.Errorf("got %q, want %q", decoded, "hello")
	}
}

func TestTryDecodeUserdata_GzipBase64(t *testing.T) {
	script := "#!/bin/bash\necho hello\n"
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	_, _ = gz.Write([]byte(script))
	_ = gz.Close()
	b64 := base64.StdEncoding.EncodeToString(buf.Bytes())

	decoded, ok := TryDecodeUserdata(b64)
	if !ok {
		t.Fatal("expected decode success")
	}
	if decoded != script {
		t.Errorf("got %q, want %q", decoded, script)
	}
}

func TestTryDecodeUserdata_Hex(t *testing.T) {
	decoded, ok := TryDecodeUserdata("68656c6c6f")
	if !ok {
		t.Fatal("expected decode success")
	}
	if decoded != "hello" {
		t.Errorf("got %q, want %q", decoded, "hello")
	}
}

func TestTryDecodeUserdata_PlainText(t *testing.T) {
	_, ok := TryDecodeUserdata("#!/bin/bash")
	if ok {
		t.Error("expected decode failure for plain text")
	}
}

func TestTryDecodeUserdata_NullBytesInOutput(t *testing.T) {
	// base64 of bytes containing null: 0x00 0x00 = "AAA="
	decoded := base64.StdEncoding.EncodeToString([]byte{0x00, 0x00, 0x00})
	_, ok := TryDecodeUserdata(decoded)
	if ok {
		t.Error("expected decode failure for null bytes in output")
	}
}

func TestTryDecodeUserdata_Empty(t *testing.T) {
	_, ok := TryDecodeUserdata("")
	if ok {
		t.Error("expected decode failure for empty string")
	}
}

func TestTryDecodeUserdata_Null(t *testing.T) {
	_, ok := TryDecodeUserdata("null")
	if ok {
		t.Error("expected decode failure for null")
	}
}

func TestTryDecodeUserdata_Sensitive(t *testing.T) {
	_, ok := TryDecodeUserdata("(sensitive value)")
	if ok {
		t.Error("expected decode failure for sensitive value")
	}
}

func TestTryDecodeUserdata_InvalidBase64(t *testing.T) {
	_, ok := TryDecodeUserdata("!!!invalid!!!")
	if ok {
		t.Error("expected decode failure for invalid base64")
	}
}

func TestTryDecodeUserdata_Base64WithSpaces(t *testing.T) {
	decoded, ok := TryDecodeUserdata("aGVs bG8=")
	if !ok {
		t.Fatal("expected decode success")
	}
	if decoded != "hello" {
		t.Errorf("got %q, want %q", decoded, "hello")
	}
}

func TestTryDecodeUserdata_OddLengthHex(t *testing.T) {
	_, ok := TryDecodeUserdata("68656c6c6f")
	if !ok {
		t.Fatal("expected decode success for valid hex")
	}
	_, ok = TryDecodeUserdata("68656c6c6")
	if ok {
		t.Error("expected decode failure for odd-length hex")
	}
}

// Test that gzip decompress failure falls back to raw base64 bytes
func TestTryDecodeUserdata_GzipMagicButInvalidGzip(t *testing.T) {
	// Bytes that start with gzip magic but are not valid gzip
	fakeGzip := []byte{0x1f, 0x8b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // invalid gzip
	b64 := base64.StdEncoding.EncodeToString(fakeGzip)
	// Should fail validation (null bytes or invalid UTF-8) - the fake gzip might decode to garbage
	// Actually the decompress might fail, so we'd use the raw bytes. Let me check - raw bytes would be
	// 0x1f 0x8b 0x00 0x00... - those contain null bytes! So validateDecoded would reject. So we'd return ("", false).
	// That's fine - we fail gracefully.
	_, ok := TryDecodeUserdata(b64)
	if ok {
		t.Error("expected decode failure for invalid gzip (contains null bytes or invalid)")
	}
}
