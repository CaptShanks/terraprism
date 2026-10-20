package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/CaptShanks/terraprism/internal/tfplan"
)

const fakeApplyScript = `#!/bin/sh
if [ -n "$FAKE_APPLY_LINES" ]; then
  old_ifs="$IFS"
  IFS='|'
  for line in $FAKE_APPLY_LINES; do
    echo "$line"
  done
  IFS="$old_ifs"
fi
exit "${FAKE_APPLY_EXIT:-0}"
`

func writeFakeApplyScript(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-apply")
	if err := os.WriteFile(path, []byte(fakeApplyScript), 0o755); err != nil {
		t.Fatalf("writing fake apply script: %v", err)
	}
	return path
}

// stepCmd runs cmd and dispatches every message it produces through
// m.Update, transparently unpacking a tea.BatchMsg (a []tea.Cmd) into its
// constituent messages first -- real tea.Program does this unpacking
// itself before a Model ever sees the individual messages, so a Model's
// own Update is never expected to handle tea.BatchMsg directly. This
// hand-driven test harness has to replicate that one step since more
// than one independent tea.Cmd can now be in flight at once (e.g. the
// spinner's own ticking alongside plan/apply progress), which is exactly
// when tea.Batch stops collapsing to a single plain command. Returns the
// updated model, the next cmd to drive (nil once everything in flight
// produced no further cmd), and every message actually dispatched, so a
// caller can check whether its target message type was among them.
func stepCmd(m Model, cmd tea.Cmd) (Model, tea.Cmd, []tea.Msg) {
	if cmd == nil {
		return m, nil, nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		newModel, nextCmd := m.Update(msg)
		return newModel.(Model), nextCmd, []tea.Msg{msg}
	}

	var msgs []tea.Msg
	var nextCmds []tea.Cmd
	for _, c := range batch {
		var subMsgs []tea.Msg
		var nextCmd tea.Cmd
		m, nextCmd, subMsgs = stepCmd(m, c)
		msgs = append(msgs, subMsgs...)
		if nextCmd != nil {
			nextCmds = append(nextCmds, nextCmd)
		}
	}
	return m, tea.Batch(nextCmds...), msgs
}

// driveApplyToCompletion runs m's already-started apply stream to
// completion by repeatedly invoking whatever tea.Cmd Update returns,
// exactly as the real Bubble Tea runtime would, and returns the final
// model once applyDoneMsg has been processed.
func driveApplyToCompletion(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; i < 1000; i++ {
		if cmd == nil {
			t.Fatalf("expected a non-nil cmd while apply is still in flight")
		}
		var msgs []tea.Msg
		m, cmd, msgs = stepCmd(m, cmd)
		for _, msg := range msgs {
			if _, ok := msg.(applyDoneMsg); ok {
				return m
			}
		}
	}
	t.Fatalf("apply did not complete within 1000 messages")
	return m
}

func TestConfirmApplyStartsStreamingAndBlocksQuit(t *testing.T) {
	fake := writeFakeApplyScript(t)
	t.Setenv("FAKE_APPLY_LINES", "creating...|still creating...|apply complete!")

	m := NewModelWithApply(simplePlan(), "/dev/null", fake, "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)
	mm.applyMode = true
	mm.confirmApply = true

	updated, cmd, _ := handleKeyConfirmApply(mm)
	if !updated.applying {
		t.Fatalf("expected applying=true immediately after confirming apply")
	}
	if updated.confirmApply {
		t.Fatalf("expected confirmApply cleared once apply starts")
	}

	// While applying, quit must be a no-op.
	quit, quitCmd, _ := handleKeyQuit(updated)
	if quitCmd != nil {
		t.Fatalf("expected quit to be blocked (nil cmd) while applying")
	}
	if !quit.applying {
		t.Fatalf("blocked quit should not otherwise change state")
	}

	final := driveApplyToCompletion(t, updated, cmd)

	if final.applying {
		t.Fatalf("expected applying=false once the stream completes")
	}
	if !final.applyAttempted {
		t.Fatalf("expected applyAttempted=true")
	}
	if final.applyResult != nil {
		t.Fatalf("expected nil applyResult on success, got %v", final.applyResult)
	}
	if !final.outputPane.Visible() {
		t.Fatalf("expected the output pane to auto-show once apply starts")
	}
	view := final.View()
	for _, want := range []string{"creating...", "still creating...", "apply complete!"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected streamed line %q visible in the output pane:\n%s", want, view)
		}
	}

	wantOutput := "creating...\nstill creating...\napply complete!\n"
	if final.ApplyOutput() != wantOutput {
		t.Fatalf("ApplyOutput() = %q, want %q", final.ApplyOutput(), wantOutput)
	}

	// Quit is allowed again now that applying has finished.
	_, quitCmd2, _ := handleKeyQuit(final)
	if quitCmd2 == nil {
		t.Fatalf("expected quit to work again once apply has finished")
	}
}

func TestApplyStreamFailureSurfacesError(t *testing.T) {
	fake := writeFakeApplyScript(t)
	t.Setenv("FAKE_APPLY_LINES", "creating...|Error: something went wrong")
	t.Setenv("FAKE_APPLY_EXIT", "1")

	m := NewModelWithApply(simplePlan(), "/dev/null", fake, "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)
	mm.applyMode = true
	mm.confirmApply = true

	updated, cmd, _ := handleKeyConfirmApply(mm)
	final := driveApplyToCompletion(t, updated, cmd)

	if final.applyResult == nil {
		t.Fatalf("expected a non-nil applyResult for a failed apply")
	}
	if final.applying {
		t.Fatalf("expected applying=false once the failed stream completes")
	}
	// ApplyOutput must be populated even on failure -- it's the whole
	// point: main.go prints it to the terminal before the failure
	// message, so the detail isn't lost when the TUI's alt-screen exits.
	wantOutput := "creating...\nError: something went wrong\n"
	if final.ApplyOutput() != wantOutput {
		t.Fatalf("ApplyOutput() = %q, want %q", final.ApplyOutput(), wantOutput)
	}
	// applyResult must stay the plain unwrapped error (not a
	// *runner.ApplyError), so the "Apply failed: %v" text doesn't
	// double-wrap into something like "Apply failed: terraform apply
	// failed: exit status 1".
	if strings.Contains(final.applyResult.Error(), "apply failed") {
		t.Fatalf("applyResult should be unwrapped, got %q", final.applyResult.Error())
	}
}

// Before the subprocess has actually started (so the exact command,
// including the generated -out=<tempfile> path, isn't known yet), the
// banner falls back to a simplified guess built from what's already
// known at construction time -- still real and specific, not a generic
// "Running plan..."/"Applying..." message.
func TestConfirmationBannerFallsBackToSimplifiedCommandBeforeStreamStarts(t *testing.T) {
	m := NewModelWithApply(simplePlan(), "/dev/null", "terraform", "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)

	mm.applying = true
	if got := mm.viewConfirmationPrompt(); !strings.Contains(got, "Running terraform apply") {
		t.Fatalf("expected apply banner to show the real command, got:\n%s", got)
	}
	mm.applying = false

	mm.planning = true
	mm.planOptions.Cmd = "tofu"
	mm.planOptions.Args = []string{"-target=aws_instance.foo"}
	if got := mm.viewConfirmationPrompt(); !strings.Contains(got, "Running tofu plan -target=aws_instance.foo") {
		t.Fatalf("expected plan banner to show the real command and args, got:\n%s", got)
	}
}

// Once the subprocess has actually started, the banner upgrades to the
// exact literal command invoked -- including internal-only flags like
// -out=<tempfile>/-no-color/-auto-approve -- not just the simplified
// pre-start guess.
func TestConfirmationBannerShowsLiteralCommandOnceStreamStarts(t *testing.T) {
	m := NewModelWithApply(simplePlan(), "/dev/null", "terraform", "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)

	mm.applying = true
	mm.applyCmdLine = "terraform apply -auto-approve /dev/null"
	if got := mm.viewConfirmationPrompt(); !strings.Contains(got, "Running terraform apply -auto-approve /dev/null") {
		t.Fatalf("expected apply banner to show the literal command, got:\n%s", got)
	}
	mm.applying = false

	mm.planning = true
	mm.planCmdLine = "tofu plan -out=/tmp/terraprism-123.tfplan -no-color -target=aws_instance.foo"
	if got := mm.viewConfirmationPrompt(); !strings.Contains(got, "Running "+mm.planCmdLine) {
		t.Fatalf("expected plan banner to show the literal command, got:\n%s", got)
	}
}

// The "Running <cmd>..." banner shows an animated spinner (one of
// spinner.Dot's frames) instead of a static hourglass, and the spinner
// keeps ticking only for as long as something is actually running --
// once planning/applying finishes, a stray in-flight tick must be a
// no-op rather than restarting the loop forever in the background.
func TestSpinnerAnimatesWhileRunningAndStopsAfter(t *testing.T) {
	m := NewModelWithApply(simplePlan(), "/dev/null", "terraform", "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)

	mm.applying = true
	mm.applyCmdLine = "terraform apply -auto-approve /dev/null"
	firstFrame := stripRenderANSI(mm.viewConfirmationPrompt())
	if strings.Contains(firstFrame, "⏳") {
		t.Fatalf("expected the static hourglass to be replaced by an animated spinner, got:\n%s", firstFrame)
	}

	updated, cmd := mm.Update(mm.spinner.Tick())
	mm = updated.(Model)
	if cmd == nil {
		t.Fatalf("expected the spinner to reschedule its own next tick while applying")
	}
	secondFrame := stripRenderANSI(mm.viewConfirmationPrompt())
	if firstFrame == secondFrame {
		t.Fatalf("expected the spinner frame to advance after a tick, got the same banner twice:\n%s", firstFrame)
	}

	mm.applying = false
	updated, cmd = mm.Update(mm.spinner.Tick())
	mm = updated.(Model)
	if cmd != nil {
		t.Fatalf("expected a spinner tick to be a no-op once nothing is running, got a rescheduled cmd")
	}
}

// A long command line (the realistic case, once the generated
// -out=<tempfile> path is included) must word-wrap onto multiple lines
// in a narrow terminal rather than being clipped by the terminal's
// right edge -- the full text must still be present somewhere in the
// banner, just spread across lines. lipgloss's Width()-driven wrap can
// break mid-token (e.g. inside "-out=") rather than only at spaces, so
// the comparison strips all whitespace from both sides instead of
// assuming wrap points land on the original string's own space
// boundaries.
func TestConfirmationBannerWordWrapsLongCommandInNarrowTerminal(t *testing.T) {
	m := NewModelWithApply(simplePlan(), "/dev/null", "terraform", "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 30})
	mm := model.(Model)

	mm.planning = true
	mm.planCmdLine = "tofu plan -out=/tmp/terraprism-987654321.tfplan -no-color -target=aws_instance.a_very_long_resource_name"
	got := stripRenderANSI(mm.viewConfirmationPrompt())

	lines := strings.Split(strings.Trim(got, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the long command to wrap onto multiple lines in a %d-wide terminal, got %d line(s):\n%s", mm.width, len(lines), got)
	}

	squash := func(s string) string {
		return strings.Join(strings.Fields(s), "")
	}
	if !strings.Contains(squash(got), squash(mm.planCmdLine)) {
		t.Fatalf("expected the full command text to survive wrapping (not be truncated), got:\n%s", got)
	}
}

func TestHandleKeyApplyRequiresConfirmation(t *testing.T) {
	m := NewModelWithApply(simplePlan(), "/dev/null", "true", "", "")
	m.applyMode = true

	updated, cmd, _ := handleKeyApply(m)
	if updated.applying || cmd != nil {
		t.Fatalf("first 'a' press should only arm confirmation, not start applying")
	}
	if !updated.confirmApply {
		t.Fatalf("expected confirmApply=true after the first 'a' press")
	}
}

// A plan with nothing to apply (e.g. "No changes. Infrastructure is
// up-to-date.") must never let 'a' arm the apply confirmation -- there's
// nothing for it to do.
func TestHandleKeyApplyDisabledWhenPlanHasNoChanges(t *testing.T) {
	m := NewModelWithApply(&tfplan.Plan{}, "/dev/null", "true", "", "")
	m.applyMode = true

	updated, cmd, _ := handleKeyApply(m)
	if updated.confirmApply || cmd != nil {
		t.Fatalf("expected 'a' to be a no-op on a plan with no changes")
	}
	if strings.Contains(m.viewHelpFooter(), "a: APPLY") {
		t.Fatalf("footer should not advertise 'a: APPLY' when the plan has no changes:\n%s", m.viewHelpFooter())
	}
}

// Once an apply has been attempted (successful or not), the plan file
// it targeted is stale -- 'a'/'y' must not be able to start a second
// apply against it. A fresh `terraprism apply` (full re-plan) is
// required instead.
func TestReApplyBlockedAfterApplyAttempted(t *testing.T) {
	fake := writeFakeApplyScript(t)
	t.Setenv("FAKE_APPLY_LINES", "apply complete!")

	m := NewModelWithApply(simplePlan(), "/dev/null", fake, "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)
	mm.applyMode = true
	mm.confirmApply = true

	updated, cmd, _ := handleKeyConfirmApply(mm)
	final := driveApplyToCompletion(t, updated, cmd)
	if !final.applyAttempted {
		t.Fatalf("setup: expected applyAttempted=true after the first apply")
	}

	afterA, cmdA, _ := handleKeyApply(final)
	if afterA.confirmApply || cmdA != nil {
		t.Fatalf("expected 'a' to be a no-op once an apply has already been attempted")
	}

	// Even if confirmApply were somehow still set, 'y' must not start a
	// second apply either.
	afterA.confirmApply = true
	afterY, cmdY, _ := handleKeyConfirmApply(afterA)
	if afterY.applying || cmdY != nil {
		t.Fatalf("expected 'y' to be a no-op once an apply has already been attempted")
	}

	if strings.Contains(final.viewHelpFooter(), "a: APPLY") {
		t.Fatalf("footer should not advertise 'a: APPLY' once an apply has already run:\n%s", final.viewHelpFooter())
	}
}

// After apply finishes, the completion banner must show a live
// countdown to auto-quit, and Esc must cancel it (leaving the banner in
// place, minus the countdown, rather than quitting or hiding it).
func TestApplyCompletionBannerCountdownAndEscCancel(t *testing.T) {
	fake := writeFakeApplyScript(t)
	t.Setenv("FAKE_APPLY_LINES", "apply complete!")

	m := NewModelWithApply(simplePlan(), "/dev/null", fake, "", "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)
	mm.applyMode = true
	mm.confirmApply = true

	updated, cmd, _ := handleKeyConfirmApply(mm)
	final := driveApplyToCompletion(t, updated, cmd)

	if final.applyQuitCountdown != applyQuitCountdownStart {
		t.Fatalf("expected countdown to start at %d, got %d", applyQuitCountdownStart, final.applyQuitCountdown)
	}
	banner := final.viewConfirmationPrompt()
	if !strings.Contains(banner, "Apply complete!") || !strings.Contains(banner, "quitting in") {
		t.Fatalf("expected completion banner with countdown, got:\n%s", banner)
	}

	cancelled, cancelCmd, _ := handleKeyEsc(final)
	if cancelCmd != nil {
		t.Fatalf("expected Esc-cancel to return a nil cmd")
	}
	if cancelled.applyQuitCountdown != 0 {
		t.Fatalf("expected Esc to zero the countdown, got %d", cancelled.applyQuitCountdown)
	}
	cancelledBanner := cancelled.viewConfirmationPrompt()
	if !strings.Contains(cancelledBanner, "Apply complete!") {
		t.Fatalf("expected the banner to remain after cancelling the countdown, got:\n%s", cancelledBanner)
	}
	if strings.Contains(cancelledBanner, "quitting in") {
		t.Fatalf("expected the countdown text gone after Esc, got:\n%s", cancelledBanner)
	}

	// A tick scheduled before the cancel must not resurrect the countdown
	// or quit once it fires.
	afterStaleTick, tickCmd := cancelled.Update(applyQuitTickMsg{})
	afterStaleTickModel := afterStaleTick.(Model)
	if afterStaleTickModel.applyQuitCountdown != 0 {
		t.Fatalf("expected a stale tick after cancellation to stay a no-op, got countdown=%d", afterStaleTickModel.applyQuitCountdown)
	}
	if tickCmd != nil {
		t.Fatalf("expected a stale tick after cancellation not to re-arm")
	}
}
