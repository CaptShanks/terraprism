package foldtree

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLogPaneHiddenByDefaultAndToggle(t *testing.T) {
	p := NewLogPane()
	if p.Visible() {
		t.Fatalf("new LogPane should start hidden")
	}
	p.SetSize(40, 5)
	p.SetLines([]string{"a", "b"})
	if p.View() != "" {
		t.Fatalf("hidden pane's View() must be empty, got %q", p.View())
	}
	p.Toggle()
	if !p.Visible() {
		t.Fatalf("Toggle should show a hidden pane")
	}
	if p.View() == "" {
		t.Fatalf("visible pane with content should render something")
	}
	p.SetVisible(false)
	if p.Visible() {
		t.Fatalf("SetVisible(false) should hide")
	}
}

func TestLogPaneSetLinesResetsToTop(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 3)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	p.SetLines(lines)
	if !p.viewport.AtTop() {
		t.Fatalf("SetLines should reset scroll to top for a document, not follow to bottom")
	}
}

func TestLogPaneAppendFollowsWhenAtBottom(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 3)
	for i := 0; i < 10; i++ {
		p.Append(fmt.Sprintf("line %d", i))
	}
	if !p.viewport.AtBottom() {
		t.Fatalf("Append should keep following (stay at bottom) when nothing has scrolled it away")
	}
	view := p.viewport.View()
	if !strings.Contains(view, "line 9") {
		t.Fatalf("expected the latest line visible while following, got:\n%s", view)
	}
}

func TestLogPaneAppendPreservesManualScrollPosition(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 3)
	for i := 0; i < 20; i++ {
		p.Append(fmt.Sprintf("line %d", i))
	}
	p.viewport.GotoTop() // user scrolls up to inspect history

	p.Append("line 20")

	if p.viewport.AtBottom() {
		t.Fatalf("appending while the user has scrolled away from the bottom must not yank them back to it")
	}
}

func TestLogPaneResetClearsContentAndScroll(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 3)
	for i := 0; i < 10; i++ {
		p.Append(fmt.Sprintf("line %d", i))
	}
	p.Reset()
	if len(p.lines) != 0 {
		t.Fatalf("Reset should clear stored lines, got %d", len(p.lines))
	}
	if !p.viewport.AtTop() {
		t.Fatalf("Reset should leave scroll at top")
	}
}

func TestLogPaneResizePreservesFollowState(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 3)
	for i := 0; i < 20; i++ {
		p.Append(fmt.Sprintf("line %d", i))
	}
	if !p.viewport.AtBottom() {
		t.Fatalf("expected to be following before resize")
	}

	p.SetSize(60, 5)
	if !p.viewport.AtBottom() {
		t.Fatalf("resizing while following should stay pinned to the new bottom")
	}

	p.viewport.GotoTop()
	p.SetSize(80, 6)
	if p.viewport.AtBottom() {
		t.Fatalf("resizing while scrolled away should not snap back to the bottom")
	}
}

func TestLogPaneNoTitleHasNoTitleBar(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 5)
	p.SetLines([]string{"a", "b"})
	p.SetVisible(true)
	if strings.Contains(p.View(), "─") {
		t.Fatalf("no title set: View() should have no rule line, got %q", p.View())
	}
	if p.Height() != 5 {
		t.Fatalf("no title set: Height() = %d, want the full 5 requested via SetSize", p.Height())
	}
}

func TestLogPaneTitleBarRendersAboveContentAndReservesOneLine(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 5)
	p.SetTitle("Plan Output")
	p.SetLines([]string{"first line"})
	p.SetVisible(true)

	if p.Height() != 4 {
		t.Fatalf("Height() = %d, want 4 (5 requested minus the title bar's own line)", p.Height())
	}

	lines := strings.Split(p.View(), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least a title line followed by content, got %d line(s): %q", len(lines), p.View())
	}
	if !strings.Contains(lines[0], "Plan Output") {
		t.Fatalf("first line = %q, want it to contain the title", lines[0])
	}
	if !strings.Contains(lines[0], "─") {
		t.Fatalf("first line = %q, want it to contain the rule character", lines[0])
	}
	if !strings.Contains(lines[1], "first line") {
		t.Fatalf("second line = %q, want the pane's actual content", lines[1])
	}
}

// SetTitle("") must give the reserved line back to the viewport, and a
// hidden pane must still render nothing regardless of title.
func TestLogPaneClearingTitleRestoresFullHeight(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 5)
	p.SetTitle("Apply Output")
	if p.Height() != 4 {
		t.Fatalf("Height() with title = %d, want 4", p.Height())
	}

	p.SetTitle("")
	if p.Height() != 5 {
		t.Fatalf("Height() after clearing title = %d, want the full 5 back", p.Height())
	}

	p.SetLines([]string{"content"})
	if p.View() != "" {
		t.Fatalf("pane is still hidden; View() should be empty regardless of title, got %q", p.View())
	}
}

// A pane too narrow to fit the title text at all must fall back to a
// plain, title-less rule rather than overflowing its own width.
func TestLogPaneTitleBarFallsBackWhenTooNarrow(t *testing.T) {
	p := NewLogPane()
	p.SetSize(5, 3)
	p.SetTitle("A Much Longer Title Than Fits")
	p.SetVisible(true)
	p.SetLines([]string{"x"})

	lines := strings.Split(p.View(), "\n")
	if len(lines) < 1 {
		t.Fatalf("expected at least the title bar line")
	}
	if strings.Contains(lines[0], "A Much Longer Title") {
		t.Fatalf("title bar should have fallen back to a plain rule when too narrow, got %q", lines[0])
	}
}

// Regression: LogPane.Update must handle tea.WindowSizeMsg itself by
// resizing the viewport (bubbles/viewport.Update never resizes on its
// own -- Width/Height are plain fields the host must set), the same way
// TreeView does. A host driving LogPane purely through the tea.Model
// interface (as SplitView/tui.Model do via a raw tea.WindowSizeMsg, not
// a direct SetSize call) would otherwise see a pane stuck at its
// zero-value size forever, rendering as empty content.
func TestLogPaneUpdateHandlesWindowSizeMsg(t *testing.T) {
	p := NewLogPane()
	p.SetVisible(true)
	m, _ := p.Update(tea.WindowSizeMsg{Width: 50, Height: 6})
	updated := m.(LogPane)
	updated.SetLines([]string{"hello"})
	if updated.View() == "" {
		t.Fatalf("expected content to render after resizing via Update(tea.WindowSizeMsg), got empty view")
	}
	if updated.viewport.Width != 50 || updated.viewport.Height != 6 {
		t.Fatalf("expected viewport resized to 50x6, got %dx%d", updated.viewport.Width, updated.viewport.Height)
	}
}

func TestLogPaneUpdateForwardsToViewport(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 3)
	for i := 0; i < 20; i++ {
		p.Append(fmt.Sprintf("line %d", i))
	}
	updatedModel, _ := p.Update(nil)
	updated := updatedModel.(LogPane)
	// A no-op message should not crash and should return a usable pane.
	if updated.viewport.Width != 40 {
		t.Fatalf("Update should preserve the underlying viewport state")
	}
}

func newTestLogPane(lines int) *LogPane {
	p := NewLogPane()
	p.SetSize(40, 3)
	content := make([]string, lines)
	for i := range content {
		content[i] = fmt.Sprintf("line %d", i)
	}
	p.SetLines(content)
	return p
}

func sendLogPaneKey(p *LogPane, s string) {
	m, _ := p.Update(keyMsgFor(s))
	*p = m.(LogPane)
}

func TestLogPaneGGAndShiftGJumpToTopAndBottom(t *testing.T) {
	p := newTestLogPane(50)
	sendLogPaneKey(p, "G")
	if !p.viewport.AtBottom() {
		t.Fatalf("expected 'G' to jump to the bottom")
	}
	sendLogPaneKey(p, "g")
	sendLogPaneKey(p, "g")
	if !p.viewport.AtTop() {
		t.Fatalf("expected 'gg' to jump to the top")
	}
}

func TestLogPaneJKForwardToViewport(t *testing.T) {
	p := newTestLogPane(50)
	before := p.viewport.YOffset
	sendLogPaneKey(p, "j")
	if p.viewport.YOffset <= before {
		t.Fatalf("expected 'j' to scroll down, offset stayed at %d", p.viewport.YOffset)
	}
}

// lineVisible reports whether line index i falls within the viewport's
// current [YOffset, YOffset+Height) window -- the right check for "did
// jumping to a match bring it into view," since SetYOffset clamps to
// maxYOffset for matches near the end of a short document (a match can
// be visible without YOffset exactly equaling its line index).
func lineVisible(p *LogPane, i int) bool {
	return i >= p.viewport.YOffset && i < p.viewport.YOffset+p.viewport.Height
}

func TestLogPaneSearchFindsAndCyclesMatches(t *testing.T) {
	p := NewLogPane()
	p.SetSize(40, 3)
	p.SetLines([]string{"alpha", "nothing here", "beta match", "more filler", "another match line", "zzz"})

	sendLogPaneKey(p, "/")
	if !p.SearchActive() {
		t.Fatalf("expected search to be active after '/'")
	}
	for _, r := range "match" {
		sendLogPaneKey(p, string(r))
	}
	if len(p.matches) != 2 {
		t.Fatalf("expected 2 matches for 'match', got %d: %v", len(p.matches), p.matches)
	}
	if !lineVisible(p, p.matches[0]) {
		t.Fatalf("expected typing to scroll the first match (line %d) into view, got YOffset=%d", p.matches[0], p.viewport.YOffset)
	}

	sendLogPaneKey(p, "enter")
	if p.SearchActive() {
		t.Fatalf("expected search input to close after enter")
	}

	sendLogPaneKey(p, "n")
	if p.matchPos != 1 || !lineVisible(p, p.matches[1]) {
		t.Fatalf("expected 'n' to move to and scroll the next match (line %d) into view, got matchPos=%d YOffset=%d", p.matches[1], p.matchPos, p.viewport.YOffset)
	}
	sendLogPaneKey(p, "n") // wraps back to the first match
	if p.matchPos != 0 || !lineVisible(p, p.matches[0]) {
		t.Fatalf("expected 'n' to wrap to the first match (line %d), got matchPos=%d YOffset=%d", p.matches[0], p.matchPos, p.viewport.YOffset)
	}
}

func TestLogPaneSearchEscClearsQueryAndMatches(t *testing.T) {
	p := newTestLogPane(20)
	sendLogPaneKey(p, "/")
	sendLogPaneKey(p, "l")
	sendLogPaneKey(p, "i")
	sendLogPaneKey(p, "n")
	sendLogPaneKey(p, "e")
	if len(p.matches) == 0 {
		t.Fatalf("setup: expected matches for 'line'")
	}
	sendLogPaneKey(p, "esc")
	if p.SearchActive() {
		t.Fatalf("expected esc to close the search input")
	}
	if p.searchQuery != "" || len(p.matches) != 0 {
		t.Fatalf("expected esc to clear the query and matches, got query=%q matches=%v", p.searchQuery, p.matches)
	}
}

func TestLogPaneViewSearchBarStates(t *testing.T) {
	p := newTestLogPane(10)
	if bar := p.ViewSearchBar(nil); bar != "" {
		t.Fatalf("expected empty search bar when not searching, got %q", bar)
	}
	sendLogPaneKey(p, "/")
	if bar := p.ViewSearchBar(nil); !strings.Contains(bar, "Search output:") {
		t.Fatalf("expected a search prompt while typing, got %q", bar)
	}
	for _, r := range "line 3" {
		sendLogPaneKey(p, string(r))
	}
	sendLogPaneKey(p, "enter")
	bar := p.ViewSearchBar(nil)
	if !strings.Contains(bar, "line 3") || !strings.Contains(bar, "1/1") {
		t.Fatalf("expected an applied-query status line with a 1/1 match count, got %q", bar)
	}
}

func TestLogPaneWordWrapIsOffByDefault(t *testing.T) {
	p := NewLogPane()
	p.SetSize(20, 5)
	if p.WordWrap() {
		t.Fatalf("expected word wrap to start disabled")
	}
	longLine := strings.Repeat("word ", 20) // ~100 chars of wrappable text, far past width 20
	p.SetLines([]string{longLine})

	if got := p.viewport.TotalLineCount(); got != 1 {
		t.Fatalf("expected the long line to stay a single row while word wrap is off, got %d rows", got)
	}
}

func TestLogPaneToggleWordWrap(t *testing.T) {
	p := NewLogPane()
	p.SetSize(20, 5)
	longLine := strings.Repeat("word ", 20)
	p.SetLines([]string{longLine})

	sendLogPaneKey(p, "w")
	if !p.WordWrap() {
		t.Fatalf("expected 'w' to enable word wrap")
	}
	if got := p.viewport.TotalLineCount(); got <= 1 {
		t.Fatalf("expected the long line to wrap into multiple displayed rows once enabled, got %d rows", got)
	}

	sendLogPaneKey(p, "w")
	if p.WordWrap() {
		t.Fatalf("expected a second 'w' to disable word wrap again")
	}
	if got := p.viewport.TotalLineCount(); got != 1 {
		t.Fatalf("expected the long line back to a single row once disabled, got %d rows", got)
	}
}

func TestLogPaneRewrapsOnWidthChangeWhenEnabled(t *testing.T) {
	p := NewLogPane()
	p.SetSize(100, 5)
	p.ToggleWordWrap()
	p.SetLines([]string{strings.Repeat("word ", 20)})
	wideRowCount := p.viewport.TotalLineCount()

	p.SetSize(20, 5)
	narrowRowCount := p.viewport.TotalLineCount()

	if narrowRowCount <= wideRowCount {
		t.Fatalf("expected narrowing the pane to increase the wrapped row count: wide=%d narrow=%d", wideRowCount, narrowRowCount)
	}
}

// ansiStrip removes ANSI escape codes, mirroring the same pattern used
// elsewhere in this codebase's tests (internal/tui/model_render_test.go)
// to compare rendered text regardless of whether color output is active
// in the current environment.
var ansiStripPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func ansiStrip(s string) string { return ansiStripPattern.ReplaceAllString(s, "") }

func TestHighlightOccurrencesPreservesTextAndMarksEveryMatch(t *testing.T) {
	line := "resource created: aws_instance.foo and aws_instance.bar"
	out := highlightOccurrences(line, "aws_instance")

	if ansiStrip(out) != line {
		t.Fatalf("highlighting must not alter the visible text:\ngot  %q\nwant %q", ansiStrip(out), line)
	}
	if strings.Count(out, "aws_instance") < 2 {
		t.Fatalf("expected both occurrences of the query still present as literal text, got %q", out)
	}
}

func TestHighlightOccurrencesIsCaseInsensitiveAndNoOpForEmptyQuery(t *testing.T) {
	if got := highlightOccurrences("Hello World", ""); got != "Hello World" {
		t.Fatalf("empty query should be a no-op, got %q", got)
	}
	out := highlightOccurrences("Hello World", "WORLD")
	if ansiStrip(out) != "Hello World" {
		t.Fatalf("case-insensitive match should still preserve original casing, got %q", ansiStrip(out))
	}
}

func TestLogPaneSearchHighlightsAppliedQueryInView(t *testing.T) {
	p := newTestLogPane(20)
	p.SetVisible(true)
	sendLogPaneKey(p, "/")
	for _, r := range "line 3" {
		sendLogPaneKey(p, string(r))
	}
	// While typing, the match should already be highlighted (and visible,
	// since typing also jumps to the first match).
	if !strings.Contains(ansiStrip(p.View()), "line 3") {
		t.Fatalf("expected the matching line visible while searching:\n%s", p.View())
	}
}

func TestLogPaneHorizontalScrollRevealsUnwrappableToken(t *testing.T) {
	p := NewLogPane()
	p.SetSize(10, 3)
	// A single token with no spaces can't be word-wrapped, so it stays as
	// one line wider than the pane -- horizontal scroll (bubbles/
	// viewport's own default "l"/"right" binding) is how the rest of it
	// becomes visible.
	token := "abcdefghijklmnopqrstuvwxyz0123456789"
	p.SetLines([]string{token})
	p.SetVisible(true)

	initial := p.View()
	if !strings.Contains(initial, "abcdefghij") {
		t.Fatalf("expected the start of the unbroken token visible initially, got %q", initial)
	}
	if strings.Contains(initial, "0123456789") {
		t.Fatalf("expected the tail of the token clipped off-screen initially, got %q", initial)
	}

	for i := 0; i < 30; i++ {
		sendLogPaneKey(p, "l")
	}
	scrolled := p.View()
	if !strings.Contains(scrolled, "0123456789") {
		t.Fatalf("expected 'l' (horizontal scroll) to eventually reveal the rest of the token, got %q", scrolled)
	}
}

// Regression: bubbles/viewport enables horizontal-scroll cropping for the
// *whole* pane the instant any single line exceeds its Width -- so if
// word wrap only soft-wrapped at spaces (wordwrap alone), one unbreakable
// token (a long ARN, a hash, ...) would silently force every other,
// already-short line into cropped/scrollable mode too. wrapLine chains a
// hard-wrap pass specifically to prevent that.
func TestLogPaneWordWrapHardBreaksUnwrappableTokensSoNoHorizontalScrollIsNeeded(t *testing.T) {
	p := NewLogPane()
	p.SetSize(10, 5)
	p.ToggleWordWrap()
	token := "abcdefghijklmnopqrstuvwxyz0123456789"
	p.SetLines([]string{token})
	p.SetVisible(true)

	view := p.View()
	for _, chunk := range []string{"abcdefghij", "klmnopqrst", "uvwxyz0123", "456789"} {
		if !strings.Contains(view, chunk) {
			t.Fatalf("expected hard-wrapped chunk %q visible without any horizontal scroll:\n%q", chunk, view)
		}
	}

	for i := 0; i < 10; i++ {
		sendLogPaneKey(p, "l")
	}
	if got := p.View(); got != view {
		t.Fatalf("expected horizontal scroll to be a no-op once word wrap guarantees every line fits:\nbefore %q\nafter  %q", view, got)
	}
}
