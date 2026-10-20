// Package runner shells out to terraform/tofu to produce a JSON plan and
// to apply it. It is the only place in terraprism that invokes the
// terraform/tofu plan/show/apply commands.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/CaptShanks/terraprism/internal/history"
	"github.com/CaptShanks/terraprism/internal/tfplan"
)

// maxScannerLineSize raises bufio.Scanner's default 64KB token limit.
// terraform/tofu output is normally line-oriented and short, but a
// single diagnostic line (e.g. a provider panic dump or an embedded
// JSON error payload) can exceed that default -- and unlike a normal
// io.Reader, exceeding it here doesn't just drop or truncate the line:
// Scan stops permanently, so the loop stops draining the pipe, which
// blocks the exec-internal goroutine copying the subprocess's real
// stdout/stderr into it, which eventually blocks the subprocess's own
// write syscall. Since quitting is disabled for the duration of a
// plan/apply, that would hang the whole TUI with no way out.
const maxScannerLineSize = 10 * 1024 * 1024

// newOutputScanner returns a line scanner over r configured with
// maxScannerLineSize, shared by PlanStream and ApplyStream.
func newOutputScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), maxScannerLineSize)
	return scanner
}

// TFCommand is the executable to invoke: "terraform", "tofu", or (in
// tests) an absolute path to a stand-in script.
type TFCommand string

// Options controls a plan invocation.
type Options struct {
	Cmd  TFCommand
	Args []string // extra tf args (-target, -var, --, etc.), not including "plan"/"-out"/"-no-color"
	Dir  string   // working directory; "" = current directory

	// KeepPlanFile, when true, leaves the binary plan file on disk and
	// reports its path in PlanResult.PlanFile so the caller can later
	// run Apply against it. When false, the plan file is removed before
	// RunPlan returns.
	KeepPlanFile bool
}

// PlanResult is the outcome of a successful RunPlan.
type PlanResult struct {
	Plan     *tfplan.Plan
	PlanFile string // "" unless Options.KeepPlanFile was set
	RawJSON  []byte
	Output   []byte // combined stdout+stderr of the `plan` invocation itself
}

// PlanError wraps a failed `plan` invocation, carrying the combined
// stdout+stderr output for display exactly as the CLI produced it.
type PlanError struct {
	Cmd    TFCommand
	Output []byte
	Err    error
}

func (e *PlanError) Error() string {
	return fmt.Sprintf("%s plan failed: %v", e.Cmd, e.Err)
}

func (e *PlanError) Unwrap() error { return e.Err }

// ShowError wraps a failed `show -json` invocation. Unlike a plan
// failure, this indicates a genuine bug or terraform/tofu incompatibility
// rather than an expected "plan had errors" case.
type ShowError struct {
	Cmd    TFCommand
	Stderr []byte
	Err    error
}

func (e *ShowError) Error() string {
	msg := fmt.Sprintf("%s show -json failed: %v", e.Cmd, e.Err)
	if len(e.Stderr) > 0 {
		msg += "\n" + string(e.Stderr)
	}
	return msg
}

func (e *ShowError) Unwrap() error { return e.Err }

// RunPlan runs `<cmd> plan -out=<tmpfile> -no-color <args...>`, then
// `<cmd> show -json <tmpfile>` and decodes the result via tfplan.Decode.
// A binary plan file is always written, since `show -json` requires one
// — there is no way to obtain JSON plan output without it.
//
// RunPlan is a thin synchronous wrapper over PlanStream (draining its
// line channel without exposing it), so a caller that doesn't need live
// progress gets identical behavior/output to one that does, by
// construction rather than by keeping two implementations in sync.
func RunPlan(ctx context.Context, opts Options) (*PlanResult, error) {
	_, lines, done := PlanStream(ctx, opts)
	for range lines {
	}
	res := <-done
	return res.Result, res.Err
}

// PlanLine is one line of streamed `plan` output.
type PlanLine struct {
	Text string
}

// PlanStreamResult is what PlanStream eventually delivers on its done
// channel: either a populated Result, or Err (a *PlanError, *ShowError,
// or plain decode error — see RunPlan's doc for what each means).
type PlanStreamResult struct {
	Result *PlanResult
	Err    error
}

// reserveTempPlanFile creates and immediately closes a uniquely-named
// plan file in the same directory internal/history manages
// (~/.terraprism/), returning its path for `plan -out=` to write into.
// Using os.CreateTemp's atomic O_EXCL creation (rather than a name
// derived only from os.Getpid, which the OS can reuse across processes)
// rules out a stale leftover ever colliding with a later run. A
// .tfplan file here is automatically invisible to `terraprism history
// list`/`view`, which only ever look at .json files.
func reserveTempPlanFile() (string, error) {
	dir, err := history.EnsureHistoryDir()
	if err != nil {
		return "", fmt.Errorf("resolving history directory: %w", err)
	}
	_, _ = history.CleanupStalePlanFiles() // best-effort; a failed sweep must never block creating a new plan file

	f, err := os.CreateTemp(dir, "terraprism-*.tfplan")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		return "", err
	}
	return path, nil
}

// PlanStream starts `<cmd> plan -out=<tmpfile> -no-color <args...>`,
// merging stdout and stderr into a single ordered stream of lines the
// same way ApplyStream does, and returns immediately once the
// subprocess has started. Lines arrive on the returned channel as
// they're produced; lines is always fully drained and closed before
// exactly one PlanStreamResult is sent on done, after which done is
// closed too.
//
// The returned cmdLine is the exact command invoked (including the
// generated -out=<tempfile> path), for a caller that wants to show the
// user what's actually running; it's "" only if reserveTempPlanFile
// itself failed, before any command could be constructed.
//
// If `plan` itself succeeds, PlanStream goes on to run `<cmd> show
// -json` (fast and not itself streamed — it's a single JSON blob, not
// line-oriented progress) and decode it before signaling done, so a
// successful PlanStreamResult always carries a fully-decoded Plan ready
// to display; a caller never sees "done" without either a usable Plan or
// an error explaining why not.
func PlanStream(ctx context.Context, opts Options) (cmdLine string, lines <-chan PlanLine, done <-chan PlanStreamResult) {
	linesCh := make(chan PlanLine, 64)
	doneCh := make(chan PlanStreamResult, 1)

	planFile, err := reserveTempPlanFile()
	if err != nil {
		close(linesCh)
		doneCh <- PlanStreamResult{Err: &PlanError{Cmd: opts.Cmd, Err: fmt.Errorf("creating temp plan file: %w", err)}}
		close(doneCh)
		return "", linesCh, doneCh
	}
	planArgs := append([]string{"plan", "-out=" + planFile, "-no-color"}, opts.Args...)
	planCmd := exec.CommandContext(ctx, string(opts.Cmd), planArgs...)
	planCmd.Dir = opts.Dir
	cmdLine = strings.Join(planCmd.Args, " ")

	pr, pw := io.Pipe()
	planCmd.Stdout = pw
	planCmd.Stderr = pw

	if err := planCmd.Start(); err != nil {
		_ = pw.Close()
		close(linesCh)
		doneCh <- PlanStreamResult{Err: &PlanError{Cmd: opts.Cmd, Err: err}}
		close(doneCh)
		return cmdLine, linesCh, doneCh
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- planCmd.Wait()
		_ = pw.Close()
	}()

	go func() {
		var output bytes.Buffer
		scanner := newOutputScanner(pr)
		for scanner.Scan() {
			text := scanner.Text()
			output.WriteString(text)
			output.WriteByte('\n')
			linesCh <- PlanLine{Text: text}
		}
		close(linesCh)

		if err := <-waitErr; err != nil {
			_ = os.Remove(planFile)
			doneCh <- PlanStreamResult{Err: &PlanError{Cmd: opts.Cmd, Output: output.Bytes(), Err: err}}
			close(doneCh)
			return
		}

		showCmd := exec.CommandContext(ctx, string(opts.Cmd), "show", "-json", planFile)
		showCmd.Dir = opts.Dir
		jsonBytes, err := showCmd.Output()
		if err != nil {
			if !opts.KeepPlanFile {
				_ = os.Remove(planFile)
			}
			var stderr []byte
			if exitErr, ok := err.(*exec.ExitError); ok {
				stderr = exitErr.Stderr
			}
			doneCh <- PlanStreamResult{Err: &ShowError{Cmd: opts.Cmd, Stderr: stderr, Err: err}}
			close(doneCh)
			return
		}

		plan, err := tfplan.DecodeBytes(jsonBytes)
		if err != nil {
			if !opts.KeepPlanFile {
				_ = os.Remove(planFile)
			}
			doneCh <- PlanStreamResult{Err: fmt.Errorf("decoding plan JSON: %w", err)}
			close(doneCh)
			return
		}

		result := &PlanResult{Plan: plan, RawJSON: jsonBytes, PlanFile: planFile, Output: output.Bytes()}
		if !opts.KeepPlanFile {
			_ = os.Remove(planFile)
			result.PlanFile = ""
		}
		doneCh <- PlanStreamResult{Result: result}
		close(doneCh)
	}()

	return cmdLine, linesCh, doneCh
}

// ApplyLine is one line of streamed `apply` output.
type ApplyLine struct {
	Text string
}

// ApplyResult is the outcome of a successful ApplyStream invocation.
type ApplyResult struct {
	Output []byte // combined stdout+stderr of the `apply` invocation itself
}

// ApplyError wraps a failed `apply` invocation, carrying the combined
// stdout+stderr output for display exactly as the CLI produced it --
// mirrors PlanError.
type ApplyError struct {
	Cmd    TFCommand
	Output []byte
	Err    error
}

func (e *ApplyError) Error() string {
	return fmt.Sprintf("%s apply failed: %v", e.Cmd, e.Err)
}

func (e *ApplyError) Unwrap() error { return e.Err }

// ApplyStreamResult is what ApplyStream eventually delivers on its done
// channel: either a populated Result, or Err (always an *ApplyError).
type ApplyStreamResult struct {
	Result *ApplyResult
	Err    error
}

// ApplyStream starts `<cmd> apply -auto-approve <planFile>`, merging
// stdout and stderr into a single ordered stream of lines the way a real
// terminal would (one process, one pipe, genuine OS-level interleaving),
// and returns immediately once the subprocess has started. Lines arrive
// on the returned channel as they're produced; lines is always fully
// drained and closed before exactly one ApplyStreamResult is sent on
// done, after which done is closed too -- so a caller doing `for range
// lines` and then reading done never misses trailing output.
//
// ApplyStreamResult carries the same combined stdout+stderr output
// either way (on Result for success, on Err's *ApplyError for failure),
// so a caller can show/print the full apply output after the fact
// without having accumulated it line-by-line itself.
//
// -auto-approve is required: unlike Apply, ApplyStream does not connect
// the subprocess's stdin to the terminal (the caller, e.g. a running
// TUI, owns the terminal instead), so terraform/tofu's own interactive
// approval prompt cannot work here — the caller is expected to have
// already gotten the user's confirmation before calling this.
//
// internal/runner takes no UI-framework dependency: pumping lines/done
// into an event loop (e.g. a Bubble Tea tea.Cmd) is the caller's job.
//
// The returned cmdLine is the exact command invoked, for a caller that
// wants to show the user what's actually running; it's always
// non-empty since, unlike PlanStream, there's no earlier step (like
// reserving a temp file) that could fail before the command itself is
// constructed.
func ApplyStream(ctx context.Context, cmd TFCommand, planFile string) (cmdLine string, lines <-chan ApplyLine, done <-chan ApplyStreamResult) {
	linesCh := make(chan ApplyLine, 64)
	doneCh := make(chan ApplyStreamResult, 1)

	applyCmd := exec.CommandContext(ctx, string(cmd), "apply", "-auto-approve", planFile)
	cmdLine = strings.Join(applyCmd.Args, " ")
	pr, pw := io.Pipe()
	applyCmd.Stdout = pw
	applyCmd.Stderr = pw

	if err := applyCmd.Start(); err != nil {
		_ = pw.Close()
		close(linesCh)
		doneCh <- ApplyStreamResult{Err: &ApplyError{Cmd: cmd, Err: err}}
		close(doneCh)
		return cmdLine, linesCh, doneCh
	}

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- applyCmd.Wait()
		_ = pw.Close()
	}()

	go func() {
		var output bytes.Buffer
		scanner := newOutputScanner(pr)
		for scanner.Scan() {
			text := scanner.Text()
			output.WriteString(text)
			output.WriteByte('\n')
			linesCh <- ApplyLine{Text: text}
		}
		close(linesCh)

		if err := <-waitErr; err != nil {
			doneCh <- ApplyStreamResult{Err: &ApplyError{Cmd: cmd, Output: output.Bytes(), Err: err}}
			close(doneCh)
			return
		}
		doneCh <- ApplyStreamResult{Result: &ApplyResult{Output: output.Bytes()}}
		close(doneCh)
	}()

	return cmdLine, linesCh, doneCh
}

// DetectCommand returns "terraform" or "tofu" based on forceTofu and
// PATH availability, preferring terraform when neither is forced.
func DetectCommand(forceTofu bool) TFCommand {
	if forceTofu {
		return TFCommand("tofu")
	}
	if _, err := exec.LookPath("terraform"); err == nil {
		return TFCommand("terraform")
	}
	if _, err := exec.LookPath("tofu"); err == nil {
		return TFCommand("tofu")
	}
	return TFCommand("terraform")
}
