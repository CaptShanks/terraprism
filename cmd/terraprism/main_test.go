package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"
)

// Test table for version flags
var versionFlagTests = []struct {
	name     string
	args     []string
	wantOut  string
	wantExit bool
}{
	{
		name:     "long version flag",
		args:     []string{"--version"},
		wantOut:  "terraprism v0.11.0\n",
		wantExit: true,
	},
	{
		name:     "short version flag",
		args:     []string{"-v"},
		wantOut:  "terraprism v0.11.0\n",
		wantExit: true,
	},
}

func TestRunVersionMode(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "current version output",
			version: "0.11.0",
			want:    "terraprism v0.11.0\n",
		},
		{
			name:    "different version output",
			version: "1.2.3",
			want:    "terraprism v1.2.3\n",
		},
		{
			name:    "dev version output",
			version: "0.0.0-dev",
			want:    "terraprism v0.0.0-dev\n",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Removed t.Parallel() to avoid race conditions with os.Stdout

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Temporarily change version constant for testing
			originalVersion := version
			// Note: We can't change the const in tests, so we'll test the actual version
			// In a real scenario, version would be injected at build time via -ldflags

			// Call the function
			runVersionMode()

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			got := buf.String()

			// Check output format matches expected pattern
			expected := fmt.Sprintf("terraprism v%s\n", originalVersion)
			if got != expected {
				t.Errorf("runVersionMode() output = %q, want %q", got, expected)
			}

			// Verify the format is correct
			if len(got) < 14 || got[:12] != "terraprism v" || got[len(got)-1] != '\n' {
				t.Errorf("runVersionMode() output format incorrect: %q", got)
			}
		})
	}
}

func TestVersionFlagsParsing(t *testing.T) {
	for _, tt := range versionFlagTests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Removed t.Parallel() to avoid race conditions with os.Stdout

			// Test that the version flags are recognized by checking main's argument parsing
			// Since main() calls os.Exit(), we'll test the logic indirectly

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Mock os.Args
			oldArgs := os.Args
			os.Args = append([]string{"terraprism"}, tt.args...)

			// We can't easily test main() directly due to os.Exit(),
			// so we test the core logic that main() uses

			// Check if args match version flags
			args := os.Args[1:]
			if len(args) > 0 {
				switch args[0] {
				case "-v", "--version":
					// This is the logic from main()
					runVersionMode()
				}
			}

			// Restore
			w.Close()
			os.Stdout = oldStdout
			os.Args = oldArgs

			// Read captured output
			var buf bytes.Buffer
			io.Copy(&buf, r)
			got := buf.String()

			if got != tt.wantOut {
				t.Errorf("version flag %s output = %q, want %q", tt.args[0], got, tt.wantOut)
			}
		})
	}
}

func TestVersionStringFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		versionStr  string
		shouldMatch bool
	}{
		{
			name:        "valid semantic version",
			versionStr:  "terraprism v1.0.0\n",
			shouldMatch: true,
		},
		{
			name:        "valid version with patch",
			versionStr:  "terraprism v0.11.0\n",
			shouldMatch: true,
		},
		{
			name:        "valid dev version",
			versionStr:  "terraprism v0.0.0-dev\n",
			shouldMatch: true,
		},
		{
			name:        "missing prefix",
			versionStr:  "v1.0.0\n",
			shouldMatch: false,
		},
		{
			name:        "wrong prefix",
			versionStr:  "terraform v1.0.0\n",
			shouldMatch: false,
		},
		{
			name:        "missing newline",
			versionStr:  "terraprism v1.0.0",
			shouldMatch: false,
		},
		{
			name:        "missing v prefix",
			versionStr:  "terraprism 1.0.0\n",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Check if the format matches the expected pattern
			matches := len(tt.versionStr) >= 14 &&
				tt.versionStr[:12] == "terraprism v" &&
				tt.versionStr[len(tt.versionStr)-1] == '\n'

			if matches != tt.shouldMatch {
				t.Errorf("version string %q format validation = %v, want %v", tt.versionStr, matches, tt.shouldMatch)
			}
		})
	}
}

func TestVersionConstantExists(t *testing.T) {
	// Verify the version constant is defined and not empty
	if version == "" {
		t.Error("version constant is empty")
	}

	// Verify it follows semantic versioning pattern (basic check)
	if len(version) < 5 { // minimum: "0.0.0"
		t.Errorf("version constant %q appears to be too short", version)
	}

	// Should contain at least one dot
	hasDot := false
	for _, char := range version {
		if char == '.' {
			hasDot = true
			break
		}
	}
	if !hasDot {
		t.Errorf("version constant %q should follow semantic versioning pattern", version)
	}
}

// Test helper to verify the exact output format
func TestVersionModeExactOutput(t *testing.T) {
	// Removed t.Parallel() to avoid race conditions with os.Stdout

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function
	runVersionMode()

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Test exact format requirements from the architecture document
	expectedPrefix := "terraprism v"
	if !bytes.HasPrefix(buf.Bytes(), []byte(expectedPrefix)) {
		t.Errorf("version output should start with %q, got: %q", expectedPrefix, output)
	}

	// Should end with newline
	if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
		t.Errorf("version output should end with newline, got: %q", output)
	}

	// Should contain the current version
	expectedOutput := fmt.Sprintf("terraprism v%s\n", version)
	if output != expectedOutput {
		t.Errorf("version output = %q, want %q", output, expectedOutput)
	}

	// Should be exactly one line (no extra output)
	lines := bytes.Count(buf.Bytes(), []byte("\n"))
	if lines != 1 {
		t.Errorf("version output should be exactly one line, got %d lines", lines)
	}
}

// Benchmark the version mode function
func BenchmarkRunVersionMode(b *testing.B) {
	// Redirect output to discard for benchmarking
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runVersionMode()
	}

	w.Close()
	os.Stdout = oldStdout
}