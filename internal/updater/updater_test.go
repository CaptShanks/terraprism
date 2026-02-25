package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurlFallbackMessage(t *testing.T) {
	msg := CurlFallbackMessage(os.ErrPermission)
	if msg == "" {
		t.Error("CurlFallbackMessage should not return empty string")
	}
	if !strings.Contains(msg, "Self-update failed") {
		t.Errorf("expected message to contain 'Self-update failed', got: %s", msg)
	}
	if !strings.Contains(msg, "curl") {
		t.Errorf("expected message to contain 'curl', got: %s", msg)
	}
	if !strings.Contains(msg, "install.sh") {
		t.Errorf("expected message to contain 'install.sh', got: %s", msg)
	}
}

func TestIsSkipUpdateCheck(t *testing.T) {
	orig := os.Getenv("TERRAPRISM_SKIP_UPDATE_CHECK")
	defer os.Setenv("TERRAPRISM_SKIP_UPDATE_CHECK", orig)

	for _, v := range []string{"1", "true", "yes", "on"} {
		os.Setenv("TERRAPRISM_SKIP_UPDATE_CHECK", v)
		if !IsSkipUpdateCheck() {
			t.Errorf("IsSkipUpdateCheck() should be true for %q", v)
		}
	}

	os.Setenv("TERRAPRISM_SKIP_UPDATE_CHECK", "0")
	if IsSkipUpdateCheck() {
		t.Error("IsSkipUpdateCheck() should be false for '0'")
	}
	os.Unsetenv("TERRAPRISM_SKIP_UPDATE_CHECK")
	if IsSkipUpdateCheck() {
		t.Error("IsSkipUpdateCheck() should be false when unset")
	}
}

func TestUpdateCheckIntervalDays(t *testing.T) {
	orig := os.Getenv("TERRAPRISM_UPDATE_CHECK_INTERVAL")
	defer os.Setenv("TERRAPRISM_UPDATE_CHECK_INTERVAL", orig)

	os.Unsetenv("TERRAPRISM_UPDATE_CHECK_INTERVAL")
	if got := UpdateCheckIntervalDays(); got != 7 {
		t.Errorf("default interval should be 7, got %d", got)
	}

	os.Setenv("TERRAPRISM_UPDATE_CHECK_INTERVAL", "14")
	if got := UpdateCheckIntervalDays(); got != 14 {
		t.Errorf("interval should be 14, got %d", got)
	}

	os.Setenv("TERRAPRISM_UPDATE_CHECK_INTERVAL", "invalid")
	if got := UpdateCheckIntervalDays(); got != 7 {
		t.Errorf("invalid interval should fallback to 7, got %d", got)
	}
}

func TestCheckLatestWithCache_NoPanic(t *testing.T) {
	// Verify CheckLatestWithCache doesn't panic; may hit network
	_, _, _ = CheckLatestWithCache("99.99.99", 7)
}

func TestCachePath(t *testing.T) {
	path, err := cachePath()
	if err != nil {
		t.Fatalf("cachePath failed: %v", err)
	}
	if path == "" {
		t.Error("cachePath should not return empty string")
	}
	if !filepath.IsAbs(path) {
		t.Errorf("cachePath should return absolute path, got: %s", path)
	}
}
