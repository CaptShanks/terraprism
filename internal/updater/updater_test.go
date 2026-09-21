package updater

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func writeUpdateCache(t *testing.T, cache updateCache) {
	t.Helper()
	path, err := cachePath()
	if err != nil {
		t.Fatalf("cachePath: %v", err)
	}
	data, err := json.Marshal(cache)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write cache: %v", err)
	}
}

func TestCheckLatestWithCacheUsesCacheWhenVersionMatches(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeUpdateCache(t, updateCache{
		LastCheckEpoch: time.Now().Unix(),
		CurrentVersion: "1.2.3",
		LatestVersion:  "9.9.9",
		HasUpdate:      true,
	})

	latest, hasUpdate, err := CheckLatestWithCache("1.2.3", 7)
	if err != nil {
		t.Fatalf("expected cache hit without error, got: %v", err)
	}
	if latest != "9.9.9" || !hasUpdate {
		t.Errorf("expected cached values (9.9.9, true), got (%q, %v)", latest, hasUpdate)
	}
}

// A cache written for one currentVersion must not be replayed for a
// different one -- otherwise bumping the local version (e.g. after a
// rebuild) keeps showing a stale "update available" nudge for the rest
// of the cache interval, since the cached HasUpdate was never true for
// the new version to begin with.
func TestCheckLatestWithCacheIgnoresCacheOnVersionMismatch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeUpdateCache(t, updateCache{
		LastCheckEpoch: time.Now().Unix(),
		CurrentVersion: "1.2.3",
		LatestVersion:  "9.9.9",
		HasUpdate:      true,
	})

	latest, hasUpdate, _ := CheckLatestWithCache("4.5.6", 7)
	if latest == "9.9.9" && hasUpdate {
		t.Errorf("expected stale cache (written for version 1.2.3) to be ignored when checking version 4.5.6, got latest=%q hasUpdate=%v", latest, hasUpdate)
	}
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
