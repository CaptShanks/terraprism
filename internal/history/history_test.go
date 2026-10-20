package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMetaEnvelopeRoundTrip(t *testing.T) {
	meta := Meta{
		Timestamp:  time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC),
		Command:    "plan",
		TFCommand:  "terraform",
		Args:       []string{"-target=module.vpc"},
		WorkingDir: "/home/user/infra",
	}
	planJSON := []byte(`{"format_version":"1.2","resource_changes":[]}`)

	envelope := FileEnvelope{Meta: meta, Plan: json.RawMessage(planJSON)}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got FileEnvelope
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !got.Meta.Timestamp.Equal(meta.Timestamp) || got.Meta.Command != meta.Command ||
		got.Meta.TFCommand != meta.TFCommand || got.Meta.WorkingDir != meta.WorkingDir {
		t.Errorf("meta round-trip mismatch: got %+v, want %+v", got.Meta, meta)
	}
	if string(got.Plan) != string(planJSON) {
		t.Errorf("plan round-trip mismatch: got %s, want %s", got.Plan, planJSON)
	}
}

func TestGenerateAndParseFilenameRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name    string
		command string
	}{
		{name: "plan", command: "plan"},
		{name: "apply", command: "apply"},
		{name: "destroy", command: "destroy"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename := GenerateFilename(tc.command)
			entry, err := parseFilename(filename)
			if err != nil {
				t.Fatalf("parseFilename(%q): %v", filename, err)
			}
			if entry.Command != tc.command {
				t.Errorf("Command = %q, want %q", entry.Command, tc.command)
			}
			if entry.Status != "" {
				t.Errorf("Status = %q, want empty", entry.Status)
			}
		})
	}
}

func TestParseFilenameWithStatus(t *testing.T) {
	entry, err := parseFilename("2026-07-24_10-00-00_myproj_apply_success.json")
	if err != nil {
		t.Fatalf("parseFilename: %v", err)
	}
	if entry.Project != "myproj" || entry.Command != "apply" || entry.Status != "success" {
		t.Errorf("got %+v", entry)
	}
	wantTime := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	if !entry.Timestamp.Equal(wantTime) {
		t.Errorf("Timestamp = %v, want %v", entry.Timestamp, wantTime)
	}
}

func TestParseFilenameRejectsUnknownCommand(t *testing.T) {
	if _, err := parseFilename("2026-07-24_10-00-00_myproj_bogus.json"); err == nil {
		t.Errorf("expected error for unknown command")
	}
}

func TestCreateHistoryFileAndListEntries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	meta := Meta{
		Timestamp:  time.Now(),
		Command:    "plan",
		TFCommand:  "terraform",
		WorkingDir: "/some/project",
	}
	planJSON := []byte(`{"format_version":"1.2","resource_changes":[]}`)

	path, err := CreateHistoryFile("plan", meta, planJSON)
	if err != nil {
		t.Fatalf("CreateHistoryFile: %v", err)
	}
	if filepath.Ext(path) != ".json" {
		t.Errorf("history file should have .json extension, got %s", path)
	}

	entries, err := ListEntries("")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	if entries[0].WorkingDir != "/some/project" {
		t.Errorf("WorkingDir = %q, want /some/project", entries[0].WorkingDir)
	}
	if entries[0].Command != "plan" {
		t.Errorf("Command = %q, want plan", entries[0].Command)
	}

	envelope, err := ReadEnvelope(path)
	if err != nil {
		t.Fatalf("ReadEnvelope: %v", err)
	}
	var got, want map[string]interface{}
	if err := json.Unmarshal(envelope.Plan, &got); err != nil {
		t.Fatalf("unmarshal got plan: %v", err)
	}
	if err := json.Unmarshal(planJSON, &want); err != nil {
		t.Fatalf("unmarshal want plan: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("Plan content mismatch: got %v, want %v", got, want)
	}
}

func TestListEntriesFilterByCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	for _, cmd := range []string{"plan", "apply", "destroy"} {
		if _, err := CreateHistoryFile(cmd, Meta{Command: cmd}, []byte(`{}`)); err != nil {
			t.Fatalf("CreateHistoryFile(%s): %v", cmd, err)
		}
	}

	entries, err := ListEntries("apply")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Command != "apply" {
		t.Fatalf("got %+v, want exactly one apply entry", entries)
	}
}

func TestUpdateFilenameWithStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	path, err := CreateHistoryFile("apply", Meta{Command: "apply"}, []byte(`{}`))
	if err != nil {
		t.Fatalf("CreateHistoryFile: %v", err)
	}

	newPath, err := UpdateFilenameWithStatus(path, StatusSuccess)
	if err != nil {
		t.Fatalf("UpdateFilenameWithStatus: %v", err)
	}
	if filepath.Ext(newPath) != ".json" {
		t.Errorf("renamed file should keep .json extension, got %s", newPath)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("renamed file should exist: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("old path should no longer exist")
	}

	entries, err := ListEntries("")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 1 || entries[0].Status != StatusSuccess {
		t.Fatalf("got %+v, want one entry with status success", entries)
	}
}

func TestCleanupStalePlanFilesRemovesOldOnesKeepsFreshOnes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir, err := EnsureHistoryDir()
	if err != nil {
		t.Fatalf("EnsureHistoryDir: %v", err)
	}

	oldPlan := filepath.Join(dir, "terraprism-old.tfplan")
	freshPlan := filepath.Join(dir, "terraprism-fresh.tfplan")
	oldJSON := filepath.Join(dir, "2020-01-01_10-00-00_myproj_plan.json")

	for _, p := range []string{oldPlan, freshPlan, oldJSON} {
		if err := os.WriteFile(p, []byte("data"), 0644); err != nil {
			t.Fatalf("writing %s: %v", p, err)
		}
	}

	staleTime := time.Now().Add(-6 * time.Hour)
	if err := os.Chtimes(oldPlan, staleTime, staleTime); err != nil {
		t.Fatalf("Chtimes oldPlan: %v", err)
	}
	if err := os.Chtimes(oldJSON, staleTime, staleTime); err != nil {
		t.Fatalf("Chtimes oldJSON: %v", err)
	}

	deleted, err := CleanupStalePlanFiles()
	if err != nil {
		t.Fatalf("CleanupStalePlanFiles: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}

	if _, err := os.Stat(oldPlan); !os.IsNotExist(err) {
		t.Errorf("old plan file should have been removed, stat err = %v", err)
	}
	if _, err := os.Stat(freshPlan); err != nil {
		t.Errorf("fresh plan file should still exist: %v", err)
	}
	if _, err := os.Stat(oldJSON); err != nil {
		t.Errorf("old .json history file should not be touched: %v", err)
	}
}

func TestCleanupStalePlanFilesNoOpWhenDirMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	deleted, err := CleanupStalePlanFiles()
	if err != nil {
		t.Fatalf("CleanupStalePlanFiles: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestListEntriesIgnoresNonJSONFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir, err := EnsureHistoryDir()
	if err != nil {
		t.Fatalf("EnsureHistoryDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2026-07-24_10-00-00_old_plan.txt"), []byte("legacy text plan"), 0644); err != nil {
		t.Fatalf("writing legacy file: %v", err)
	}

	entries, err := ListEntries("")
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("legacy .txt history files should be ignored, got %d entries", len(entries))
	}
}
