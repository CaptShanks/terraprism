package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/rhysd/go-github-selfupdate/selfupdate"
)

const (
	repoSlug        = "CaptShanks/terraprism"
	installScriptURL = "https://raw.githubusercontent.com/CaptShanks/terraprism/main/install.sh"
)

// CheckLatest fetches the latest release from GitHub and compares with currentVersion.
// Returns (latestVersion, hasUpdate, err). Never blocks or fails the main command on errors.
func CheckLatest(currentVersion string) (latestVersion string, hasUpdate bool, err error) {
	latest, found, err := selfupdate.DetectLatest(repoSlug)
	if err != nil || !found {
		return "", false, err
	}
	latestVersion = latest.Version.String()
	// Strip 'v' prefix for comparison if present in tag
	latestVersion = strings.TrimPrefix(latestVersion, "v")

	current := normalizeVersion(currentVersion)
	latestSemver, err := semver.Parse(latestVersion)
	if err != nil {
		return latestVersion, false, err
	}
	currentSemver, err := semver.Parse(current)
	if err != nil {
		return latestVersion, false, err
	}
	hasUpdate = latestSemver.GT(currentSemver)
	return latestVersion, hasUpdate, nil
}

// Upgrade replaces the current binary with the latest release.
// On success returns the new version. On failure returns an error suitable for displaying
// the curl fallback command.
func Upgrade(currentVersion string) (newVersion string, err error) {
	current := normalizeVersion(currentVersion)
	v, err := semver.Parse(current)
	if err != nil {
		return "", fmt.Errorf("invalid version %q: %w", currentVersion, err)
	}

	latest, err := selfupdate.UpdateSelf(v, repoSlug)
	if err != nil {
		return "", err
	}
	return latest.Version.String(), nil
}

// CurlFallbackMessage returns the message to display when self-update fails.
func CurlFallbackMessage(reason error) string {
	return fmt.Sprintf(`Self-update failed: %v
To upgrade manually, run:
  curl -sSfL %s | sh`, reason, installScriptURL)
}

func normalizeVersion(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "v") {
		return s[1:]
	}
	return s
}

// updateCache holds cached update check results.
type updateCache struct {
	LastCheckEpoch  int64  `json:"last_check_epoch"`
	LatestVersion   string `json:"latest_version,omitempty"`
	HasUpdate       bool   `json:"has_update"`
}

// cachePath returns the path to the update check cache file.
func cachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".terraprism")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "update-check"), nil
}

// CheckLatestWithCache checks for updates, but only if the cache interval has elapsed.
// intervalDays is the number of days between checks (default 7).
// Returns (latestVersion, hasUpdate, err). If within interval, uses cached result.
func CheckLatestWithCache(currentVersion string, intervalDays int) (latestVersion string, hasUpdate bool, err error) {
	if intervalDays <= 0 {
		intervalDays = 7
	}
	intervalSec := int64(intervalDays) * 24 * 60 * 60

	path, err := cachePath()
	if err != nil {
		return CheckLatest(currentVersion)
	}

	// Read cache
	data, err := os.ReadFile(path)
	if err == nil {
		var cache updateCache
		if json.Unmarshal(data, &cache) == nil {
			now := time.Now().Unix()
			if now-cache.LastCheckEpoch < intervalSec {
				return cache.LatestVersion, cache.HasUpdate, nil
			}
		}
	}

	// Fetch from API
	latest, hasUpdate, err := CheckLatest(currentVersion)
	if err != nil {
		return "", false, err
	}

	// Write cache
	cache := updateCache{
		LastCheckEpoch: time.Now().Unix(),
		LatestVersion:  latest,
		HasUpdate:      hasUpdate,
	}
	if data, err := json.Marshal(cache); err == nil {
		_ = os.WriteFile(path, data, 0644)
	}

	return latest, hasUpdate, nil
}

// IsSkipUpdateCheck returns true if TERRAPRISM_SKIP_UPDATE_CHECK is set.
func IsSkipUpdateCheck() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TERRAPRISM_SKIP_UPDATE_CHECK")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// UpdateCheckIntervalDays returns the configured interval in days (default 7).
func UpdateCheckIntervalDays() int {
	v := strings.TrimSpace(os.Getenv("TERRAPRISM_UPDATE_CHECK_INTERVAL"))
	if v == "" {
		return 7
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 7
	}
	return n
}
