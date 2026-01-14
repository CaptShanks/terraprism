// Package history manages the storage and retrieval of plan/apply output files.
package history

import (
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
)

// Entry represents a history file entry
type Entry struct {
	Path      string
	Timestamp time.Time
	Project   string // directory/project name
	Command   string // plan, apply, destroy
	Status    string // pending, success, failed, cancelled (for apply/destroy)
	Filename  string
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
// Format: YYYY-MM-DD_HH-MM-SS_<project>_<command>.txt
func GenerateFilename(command string) string {
	now := time.Now()
	project := sanitizeProjectName(GetWorkingDir())
	return fmt.Sprintf("%s_%s_%s.txt",
		now.Format("2006-01-02_15-04-05"),
		project,
		command,
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

// CreateHistoryFile creates a new history file and returns its path
func CreateHistoryFile(command string, content string) (string, error) {
	dir, err := EnsureHistoryDir()
	if err != nil {
		return "", err
	}

	filename := GenerateFilename(command)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write history file: %w", err)
	}

	return path, nil
}

// AppendToHistoryFile appends content to an existing history file
func AppendToHistoryFile(path string, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString(content); err != nil {
		return fmt.Errorf("failed to append to history file: %w", err)
	}

	return nil
}

// UpdateFilenameWithStatus renames a history file to include the status
// e.g., 2024-01-09_10-30-00_apply.txt -> 2024-01-09_10-30-00_apply_success.txt
func UpdateFilenameWithStatus(oldPath string, status string) (string, error) {
	dir := filepath.Dir(oldPath)
	filename := filepath.Base(oldPath)

	// Remove .txt extension
	base := strings.TrimSuffix(filename, ".txt")

	// Add status suffix
	newFilename := fmt.Sprintf("%s_%s.txt", base, status)
	newPath := filepath.Join(dir, newFilename)

	if err := os.Rename(oldPath, newPath); err != nil {
		return "", fmt.Errorf("failed to rename history file: %w", err)
	}

	return newPath, nil
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
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".txt") {
			continue
		}

		entry, err := parseFilename(f.Name())
		if err != nil {
			continue // Skip files that don't match our format
		}

		entry.Path = filepath.Join(dir, f.Name())
		entry.Filename = f.Name()

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

// parseFilename parses a history filename into an Entry
// New format: YYYY-MM-DD_HH-MM-SS_<project>_<command>[_<status>].txt
// Old format: YYYY-MM-DD_HH-MM-SS_<command>[_<status>].txt (for backwards compatibility)
func parseFilename(filename string) (Entry, error) {
	base := strings.TrimSuffix(filename, ".txt")
	parts := strings.Split(base, "_")

	if len(parts) < 3 {
		return Entry{}, fmt.Errorf("invalid filename format")
	}

	// Parse timestamp (first two parts: date and time)
	dateStr := parts[0]
	timeStr := parts[1]
	timestamp, err := time.Parse("2006-01-02_15-04-05", dateStr+"_"+timeStr)
	if err != nil {
		return Entry{}, fmt.Errorf("invalid timestamp: %w", err)
	}

	knownCommands := map[string]bool{"plan": true, "apply": true, "destroy": true}

	var project, command, status string

	// Determine format based on number of parts and content
	// 3 parts: old format without status (date_time_command)
	// 4 parts: could be old+status OR new without status - check if parts[3] is a command
	// 5+ parts: new format with status (date_time_project_command_status)
	switch len(parts) {
	case 3:
		// Old format: date_time_command
		project = ""
		command = parts[2]
		status = ""

	case 4:
		// Ambiguous: could be old+status OR new without status
		// If parts[3] is a known command, it's new format (project_command)
		// Otherwise it's old format (command_status)
		if knownCommands[parts[3]] {
			// New format: date_time_project_command
			project = parts[2]
			command = parts[3]
			status = ""
		} else {
			// Old format: date_time_command_status
			project = ""
			command = parts[2]
			status = parts[3]
		}

	default: // 5+ parts
		// New format: date_time_project_command_status
		project = parts[2]
		command = parts[3]
		status = parts[4]
	}

	// Validate command is known
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

// FormatEntry formats an entry for display
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

// GetWorkingDir returns the current working directory name for context
func GetWorkingDir() string {
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return filepath.Base(wd)
}

// CreateHistoryHeader creates a header for the history file
func CreateHistoryHeader(command string, tfCmd string, args []string) string {
	wd := GetWorkingDir()
	now := time.Now()

	header := fmt.Sprintf(`================================================================================
Terra-Prism History Log
================================================================================
Timestamp:   %s
Command:     %s %s
Working Dir: %s
Arguments:   %s
================================================================================

`, now.Format("2006-01-02 15:04:05 MST"),
		tfCmd, command,
		wd,
		strings.Join(args, " "),
	)

	return header
}

// CreateApplyResultFooter creates a footer with apply results
func CreateApplyResultFooter(success bool, err error) string {
	now := time.Now()
	status := "SUCCESS"
	errMsg := ""
	if !success {
		status = "FAILED"
		if err != nil {
			errMsg = fmt.Sprintf("\nError: %v", err)
		}
	}

	return fmt.Sprintf(`
================================================================================
Apply Result: %s
Completed:    %s%s
================================================================================
`, status, now.Format("2006-01-02 15:04:05 MST"), errMsg)
}
