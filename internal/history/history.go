// Package history manages the storage and retrieval of plan/apply output files.
package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// HistoryDir is the directory name for storing history files
	HistoryDir = ".terraprism"

	// StatusPending indicates apply hasn't completed yet
	StatusPending = "pending"
	// StatusSuccess indicates apply succeeded
	StatusSuccess = "success"
	// StatusFailed indicates apply failed
	StatusFailed = "failed"
	// StatusCancelled indicates apply was cancelled
	StatusCancelled = "cancelled"

	// MaxHistoryFiles is the maximum number of history files to keep
	// Older files are automatically cleaned up
	MaxHistoryFiles = 100

	fileExt = ".json"

	// stalePlanFileAge is how old an orphaned interim plan file has to be
	// before CleanupStalePlanFiles removes it. terraprism can run multiple
	// sessions in parallel, each with its own plan file live in
	// ~/.terraprism/ at the same time, so this only ever removes files far
	// older than any realistic single plan/apply review session -- a
	// concurrent session's own in-progress file is always much younger and
	// is never at risk. A normal plan/apply cycle finishes in minutes and
	// always cleans up its own plan file either way; this is a backstop
	// for the one case code can't guarantee against -- a hard kill
	// (SIGKILL) that skips cleanup entirely.
	stalePlanFileAge = 5 * time.Hour
)

// Entry represents a history file entry
type Entry struct {
	Path       string
	Timestamp  time.Time
	Project    string // directory/project name
	Command    string // plan, apply, destroy
	Status     string // pending, success, failed, cancelled (for apply/destroy)
	Filename   string
	WorkingDir string // full absolute path of terraform project
}

// Meta is the terraprism-owned metadata stored alongside each plan in a
// history file's envelope.
type Meta struct {
	Timestamp  time.Time `json:"timestamp"`
	Command    string    `json:"command"`    // "plan", "apply", or "destroy"
	TFCommand  string    `json:"tf_command"` // "terraform" or "tofu"
	Args       []string  `json:"args"`
	WorkingDir string    `json:"working_dir"`
}

// FileEnvelope is the on-disk shape of a history file: terraprism's own
// metadata alongside the raw `terraform show -json` plan bytes, stored
// verbatim so it stays decodable by any future tfplan.Decode version.
type FileEnvelope struct {
	Meta Meta            `json:"terraprism_meta"`
	Plan json.RawMessage `json:"plan"`
}

// GetHistoryDir returns the path to the history directory
func GetHistoryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, HistoryDir), nil
}

// EnsureHistoryDir creates the history directory if it doesn't exist
func EnsureHistoryDir() (string, error) {
	dir, err := GetHistoryDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create history directory: %w", err)
	}

	return dir, nil
}

// GenerateFilename creates a filename for a history entry
// Format: YYYY-MM-DD_HH-MM-SS_<project>_<command>.json
func GenerateFilename(command string) string {
	now := time.Now()
	project := sanitizeProjectName(GetWorkingDir())
	return fmt.Sprintf("%s_%s_%s%s",
		now.Format("2006-01-02_15-04-05"),
		project,
		command,
		fileExt,
	)
}

// sanitizeProjectName makes a project name safe for filenames
// Underscores MUST be replaced since they're used as filename delimiters
func sanitizeProjectName(name string) string {
	// Replace problematic characters with dashes
	// IMPORTANT: underscores are filename delimiters, so they must be replaced
	replacer := strings.NewReplacer(
		"_", "-",
		" ", "-",
		"/", "-",
		"\\", "-",
		":", "-",
		".", "-",
	)
	name = replacer.Replace(name)

	// Limit length to keep filenames reasonable
	if len(name) > 30 {
		name = name[:30]
	}

	// Prevent project names that match command names (would confuse parser)
	knownCommands := map[string]bool{"plan": true, "apply": true, "destroy": true}
	if knownCommands[name] {
		name = name + "-proj"
	}

	return name
}

// CreateHistoryFile writes a new history file (metadata envelope + raw
// plan JSON) and returns its path.
func CreateHistoryFile(command string, meta Meta, planJSON []byte) (string, error) {
	dir, err := EnsureHistoryDir()
	if err != nil {
		return "", err
	}

	filename := GenerateFilename(command)
	path := filepath.Join(dir, filename)

	envelope := FileEnvelope{Meta: meta, Plan: json.RawMessage(planJSON)}
	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal history envelope: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write history file: %w", err)
	}

	return path, nil
}

// ReadEnvelope reads and decodes a history file's envelope.
func ReadEnvelope(path string) (*FileEnvelope, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}
	var envelope FileEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse history file: %w", err)
	}
	return &envelope, nil
}

// UpdateFilenameWithStatus renames a history file to include the status,
// e.g. 2024-01-09_10-30-00_myproj_apply.json -> ..._apply_success.json
func UpdateFilenameWithStatus(oldPath string, status string) (string, error) {
	dir := filepath.Dir(oldPath)
	filename := filepath.Base(oldPath)

	base := strings.TrimSuffix(filename, fileExt)
	newFilename := fmt.Sprintf("%s_%s%s", base, status, fileExt)
	newPath := filepath.Join(dir, newFilename)

	if err := os.Rename(oldPath, newPath); err != nil {
		return "", fmt.Errorf("failed to rename history file: %w", err)
	}

	return newPath, nil
}

// readMeta reads just the terraprism_meta block of a history file. It
// streams the file token-by-token and stops as soon as terraprism_meta
// has been decoded, so it never reads the (potentially many hundreds of
// MB) plan payload that follows it into memory -- unlike os.ReadFile +
// json.Unmarshal, which must read and scan the entire file regardless of
// which fields are actually needed. This matters because readMeta runs
// once per history file on every plan/apply invocation (via
// CleanupOldFiles), so its cost is otherwise multiplied by however many
// history files have accumulated.
func readMeta(path string) Meta {
	f, err := os.Open(path)
	if err != nil {
		return Meta{}
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	if _, err := dec.Token(); err != nil { // consume opening '{'
		return Meta{}
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return Meta{}
		}
		if key, _ := keyTok.(string); key == "terraprism_meta" {
			var meta Meta
			if err := dec.Decode(&meta); err != nil {
				return Meta{}
			}
			return meta
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return Meta{}
		}
	}
	return Meta{}
}

// ListEntries returns all history entries, sorted by timestamp (newest first)
func ListEntries(filterCommand string) ([]Entry, error) {
	dir, err := GetHistoryDir()
	if err != nil {
		return nil, err
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []Entry{}, nil
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read history directory: %w", err)
	}

	var entries []Entry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), fileExt) {
			continue
		}

		entry, err := parseFilename(f.Name())
		if err != nil {
			continue // Skip files that don't match our format
		}

		entry.Path = filepath.Join(dir, f.Name())
		entry.Filename = f.Name()
		entry.WorkingDir = readMeta(entry.Path).WorkingDir

		// Filter by command if specified
		if filterCommand != "" && entry.Command != filterCommand {
			continue
		}

		entries = append(entries, entry)
	}

	// Sort by timestamp, newest first
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	return entries, nil
}

// CleanupOldFiles removes old history files if count exceeds MaxHistoryFiles
func CleanupOldFiles() (int, error) {
	entries, err := ListEntries("")
	if err != nil {
		return 0, err
	}

	if len(entries) <= MaxHistoryFiles {
		return 0, nil
	}

	// Entries are sorted newest first, so delete from the end
	deleted := 0
	for i := MaxHistoryFiles; i < len(entries); i++ {
		if err := os.Remove(entries[i].Path); err == nil {
			deleted++
		}
	}

	return deleted, nil
}

// CleanupStalePlanFiles removes any interim binary plan file
// (terraform plan -out=<this>) left behind in the history directory by
// a process that was killed before it could clean up after itself.
// Safe to call on every new plan invocation (see internal/runner) --
// most of the time there's nothing to remove.
func CleanupStalePlanFiles() (int, error) {
	dir, err := GetHistoryDir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := time.Now().Add(-stalePlanFileAge)
	deleted := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "terraprism-") || !strings.HasSuffix(name, ".tfplan") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err == nil {
			deleted++
		}
	}
	return deleted, nil
}

// parseFilename parses a history filename into an Entry.
// Format: YYYY-MM-DD_HH-MM-SS_<project>_<command>[_<status>].json
func parseFilename(filename string) (Entry, error) {
	base := strings.TrimSuffix(filename, fileExt)
	parts := strings.Split(base, "_")

	if len(parts) < 4 {
		return Entry{}, fmt.Errorf("invalid filename format")
	}

	dateStr := parts[0]
	timeStr := parts[1]
	timestamp, err := time.Parse("2006-01-02_15-04-05", dateStr+"_"+timeStr)
	if err != nil {
		return Entry{}, fmt.Errorf("invalid timestamp: %w", err)
	}

	knownCommands := map[string]bool{"plan": true, "apply": true, "destroy": true}

	project := parts[2]
	command := parts[3]
	status := ""
	if len(parts) >= 5 {
		status = parts[4]
	}

	if !knownCommands[command] {
		return Entry{}, fmt.Errorf("unknown command: %s", command)
	}

	return Entry{
		Timestamp: timestamp,
		Project:   project,
		Command:   command,
		Status:    status,
	}, nil
}

// TruncatePath truncates a path from the left, keeping the rightmost portion
func TruncatePath(path string, maxLen int) string {
	if len(path) <= maxLen {
		return path
	}
	// Keep the rightmost portion with "..." prefix
	return "..." + path[len(path)-maxLen+3:]
}

// FormatEntry formats an entry for display (basic format without path)
func FormatEntry(e Entry) string {
	status := ""
	if e.Status != "" {
		switch e.Status {
		case StatusSuccess:
			status = "[SUCCESS]"
		case StatusFailed:
			status = "[FAILED]"
		case StatusCancelled:
			status = "[CANCELLED]"
		case StatusPending:
			status = "[PENDING]"
		}
	}

	project := e.Project
	if project == "" {
		project = "-"
	}
	// Truncate long project names for display
	if len(project) > 20 {
		project = project[:17] + "..."
	}

	return fmt.Sprintf("%s  %-20s  %-8s  %-12s",
		e.Timestamp.Format("2006-01-02 15:04:05"),
		project,
		e.Command,
		status,
	)
}

// FormatEntryWithPath formats an entry with the working directory path
func FormatEntryWithPath(e Entry) string {
	status := ""
	if e.Status != "" {
		switch e.Status {
		case StatusSuccess:
			status = "[SUCCESS]"
		case StatusFailed:
			status = "[FAILED]"
		case StatusCancelled:
			status = "[CANCELLED]"
		case StatusPending:
			status = "[PENDING]"
		}
	}

	path := e.WorkingDir
	if path == "" {
		path = "-"
	}
	path = TruncatePath(path, 40)

	return fmt.Sprintf("%s  %-7s  %-12s  %s",
		e.Timestamp.Format("2006-01-02 15:04"),
		e.Command,
		status,
		path,
	)
}

// GetWorkingDir returns the current working directory basename for context
func GetWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return filepath.Base(wd)
}

// GetFullWorkingDir returns the full absolute path of the current working directory
func GetFullWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return wd
}
