package foldtree

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// testItem is a synthetic, non-Terraform payload used to prove TreeView
// works generically: it implements Searchable so it can opt into search.
type testItem struct {
	label string
}

func (i testItem) SearchText() string { return i.label }

// testRenderer is a minimal RowRenderer: each row renders as its ID,
// prefixed with ">" when selected, so tests can assert on plain text
// without depending on any styling.
type testRenderer struct{}

func (testRenderer) RenderRow(row Row, selected bool, width int, searchQuery string) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}
	return prefix + row.ID
}

func (testRenderer) EmptyMessage(query string) string {
	if query != "" {
		return "no matches for " + query
	}
	return "empty"
}

func testTree() []Node {
	return []Node{
		{ID: "alpha", Height: 1, Collapsible: true, Payload: testItem{"alpha"},
			Children: []Node{{ID: "alpha.1", Height: 1}}},
		{ID: "beta", Height: 1, Collapsible: true, Payload: testItem{"beta"},
			Children: []Node{{ID: "beta.1", Height: 1}}},
		{ID: "gamma", Height: 1, Payload: testItem{"gamma"}},
	}
}

func newTestTreeView(width, height int) *TreeView {
	tv := NewTreeView(testRenderer{})
	tv.SetTree(testTree())
	m, _ := tv.Update(tea.WindowSizeMsg{Width: width, Height: height})
	*tv = m.(TreeView)
	return tv
}

func sendKey(tv *TreeView, s string) {
	m, _ := tv.Update(keyMsgFor(s))
	*tv = m.(TreeView)
}

func keyMsgFor(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// TreeView itself has no notion of a "default collapsed" state -- that's
// a domain policy (e.g. terraprism collapsing resources by default) that
// stays with the host, applied via State().SetCollapsed after SetTree.
// A bare TreeView shows every node expanded until told otherwise.
func TestTreeViewRendersAllRowsExpandedByDefault(t *testing.T) {
	tv := newTestTreeView(40, 10)
	view := tv.View()
	for _, want := range []string{"alpha", "alpha.1", "beta", "beta.1", "gamma"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected %q rendered by default, got:\n%s", want, view)
		}
	}
}

func TestTreeViewEnterTogglesCollapse(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "enter") // cursor starts on "alpha"; collapse it
	view := tv.View()
	if strings.Contains(view, "alpha.1") {
		t.Fatalf("expected child hidden after toggling collapse, got:\n%s", view)
	}
	sendKey(tv, "enter") // expand again
	if !strings.Contains(tv.View(), "alpha.1") {
		t.Fatalf("expected child visible after toggling collapse again")
	}
}

func TestTreeViewUpDownMovesSelection(t *testing.T) {
	tv := newTestTreeView(40, 10)
	id0, _ := tv.nav.SelectedID()
	sendKey(tv, "down")
	id1, _ := tv.nav.SelectedID()
	if id0 == id1 {
		t.Fatalf("expected selection to move on 'down', stayed at %q", id0)
	}
	sendKey(tv, "up")
	id2, _ := tv.nav.SelectedID()
	if id2 != id0 {
		t.Fatalf("expected 'up' to return to %q, got %q", id0, id2)
	}
}

func TestTreeViewMouseWheelScrolls(t *testing.T) {
	tv := newTestTreeView(40, 2) // small viewport forces scrolling to matter
	before := tv.nav.Offset()
	m, _ := tv.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	*tv = m.(TreeView)
	if tv.nav.Offset() == before && tv.nav.SelectedIndex() == 0 {
		// Only assert movement happened somehow (offset or selection);
		// exact scroll math is foldtree's own State's responsibility.
		t.Fatalf("expected wheel-down to move something (offset or selection)")
	}
}

// The horizontal wheel (a trackpad swipe or shift+wheel) is reported as
// its own button, not a modifier on WheelUp/Down -- same escape valve
// as the H/L keys, just via the mouse.
func TestTreeViewMouseWheelScrollsHorizontally(t *testing.T) {
	longID := "abcdefghijklmnopqrstuvwxyz0123456789"
	tv := NewTreeView(testRenderer{})
	tv.SetTree([]Node{{ID: longID, Height: 1, Payload: testItem{longID}}})
	m, _ := tv.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	*tv = m.(TreeView)

	for i := 0; i < 30; i++ {
		m, _ = tv.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelRight})
		*tv = m.(TreeView)
	}
	scrolled := tv.View()
	if !strings.Contains(scrolled, "0123456789") {
		t.Fatalf("expected wheel-right to eventually reveal the rest of the row, got %q", scrolled)
	}

	for i := 0; i < 30; i++ {
		m, _ = tv.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelLeft})
		*tv = m.(TreeView)
	}
	back := tv.View()
	if !strings.Contains(back, "abcdefgh") {
		t.Fatalf("expected wheel-left to scroll back to the start, got %q", back)
	}
}

func TestTreeViewSearchNarrowsToMatchingTopLevelNodes(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "/")
	if !tv.SearchActive() {
		t.Fatalf("expected search to be active after '/'")
	}
	for _, r := range "beta" {
		sendKey(tv, string(r))
	}
	if tv.SearchQuery() != "beta" {
		t.Fatalf("SearchQuery() = %q, want %q", tv.SearchQuery(), "beta")
	}
	view := tv.View()
	if strings.Contains(view, "alpha") || strings.Contains(view, "gamma") {
		t.Fatalf("expected non-matching top-level rows filtered out, got:\n%s", view)
	}
	if !strings.Contains(view, "beta") {
		t.Fatalf("expected the matching row still shown, got:\n%s", view)
	}
}

func TestTreeViewSearchEscClearsAndRestoresFullTree(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "/")
	sendKey(tv, "b")
	sendKey(tv, "e")
	sendKey(tv, "t")
	sendKey(tv, "a")
	sendKey(tv, "enter") // apply, stop typing
	if tv.SearchActive() {
		t.Fatalf("expected search input to close after enter")
	}
	sendKey(tv, "esc") // clear the applied query (not typing anymore)
	if tv.SearchQuery() != "" {
		t.Fatalf("expected query cleared after esc, got %q", tv.SearchQuery())
	}
	view := tv.View()
	if !strings.Contains(view, "alpha") || !strings.Contains(view, "gamma") {
		t.Fatalf("expected full tree restored after clearing search, got:\n%s", view)
	}
}

func TestTreeViewEmptyMessageShownWhenSearchMatchesNothing(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "/")
	for _, r := range "zzz" {
		sendKey(tv, string(r))
	}
	view := tv.View()
	if !strings.Contains(view, "no matches for zzz") {
		t.Fatalf("expected renderer's EmptyMessage for the query, got:\n%s", view)
	}
}

func TestTreeViewSetTreeReappliesActiveSearch(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "/")
	for _, r := range "beta" {
		sendKey(tv, string(r))
	}
	sendKey(tv, "enter")

	// Host rebuilds/re-sorts independently of the search (e.g. a filter
	// or sort toggle) -- SetTree must re-narrow to the still-active query
	// rather than showing the caller's full new tree unfiltered.
	tv.SetTree(testTree())

	view := tv.View()
	if strings.Contains(view, "alpha") || strings.Contains(view, "gamma") {
		t.Fatalf("expected SetTree to keep applying the active search query, got:\n%s", view)
	}
	if !strings.Contains(view, "beta") {
		t.Fatalf("expected the matching row still shown after SetTree, got:\n%s", view)
	}
}

func TestTreeViewExtraPaddingAppearsAfterRows(t *testing.T) {
	tv := newTestTreeView(40, 10)
	tv.SetExtraPadding(3)
	lines := strings.Split(tv.render(), "\n")
	trailingBlank := 0
	for i := len(lines) - 1; i >= 0 && lines[i] == ""; i-- {
		trailingBlank++
	}
	if trailingBlank < 3 {
		t.Fatalf("expected at least 3 trailing blank lines from SetExtraPadding(3), got %d in:\n%q", trailingBlank, lines)
	}
}

func TestTreeViewViewSearchBarStates(t *testing.T) {
	tv := newTestTreeView(40, 10)
	if bar := tv.ViewSearchBar(nil); bar != "" {
		t.Fatalf("expected empty search bar when not searching, got %q", bar)
	}
	sendKey(tv, "/")
	if bar := tv.ViewSearchBar(nil); !strings.Contains(bar, "Search:") {
		t.Fatalf("expected a search prompt while typing, got %q", bar)
	}
	for _, r := range "beta" {
		sendKey(tv, string(r))
	}
	sendKey(tv, "enter")
	bar := tv.ViewSearchBar(nil)
	if !strings.Contains(bar, "beta") || !strings.Contains(bar, "1/1") {
		t.Fatalf("expected an applied-query status line with a 1/1 match count, got %q", bar)
	}
}

func TestTreeViewExpandCollapseCurrentKeys(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "l") // expand current (alpha)
	if !strings.Contains(tv.View(), "alpha.1") {
		t.Fatalf("expected 'l' to expand the current node")
	}
	sendKey(tv, "h") // collapse current
	if strings.Contains(tv.View(), "alpha.1") {
		t.Fatalf("expected 'h' to collapse the current node")
	}
}

// A leaf has nothing of its own to collapse, so 'c' must climb to its
// parent and collapse that branch instead -- otherwise 'c' silently
// does nothing whenever the cursor sits on a leaf, which is most of the
// time in a real attribute tree.
func TestTreeViewCKeyOnLeafCollapsesParent(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "down") // alpha -> alpha.1 (a leaf)
	if id, _ := tv.nav.SelectedID(); id != "alpha.1" {
		t.Fatalf("setup: expected selection on alpha.1, got %q", id)
	}

	sendKey(tv, "c")

	if id, _ := tv.nav.SelectedID(); id != "alpha" {
		t.Fatalf("expected 'c' on a leaf to move selection to its parent (alpha), got %q", id)
	}
	if strings.Contains(tv.View(), "alpha.1") {
		t.Fatalf("expected 'c' on a leaf to collapse its parent, hiding alpha.1:\n%s", tv.View())
	}
}

// 'c' on a root row with no parent to climb to (and nothing of its own
// to collapse) falls back to collapsing the whole tree, matching 'C'.
func TestTreeViewCKeyOnRootLeafCollapsesAll(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "G") // jump to gamma, a root leaf

	sendKey(tv, "c")

	view := tv.View()
	if strings.Contains(view, "alpha.1") || strings.Contains(view, "beta.1") {
		t.Fatalf("expected 'c' on a root leaf to collapse the whole tree:\n%s", view)
	}
}

func TestTreeViewGGTopAndShiftGBottom(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "down")
	sendKey(tv, "down")
	sendKey(tv, "G")
	if id, _ := tv.nav.SelectedID(); id != "gamma" {
		t.Fatalf("expected 'G' to jump to the last row (gamma), got %q", id)
	}
	sendKey(tv, "g")
	sendKey(tv, "g")
	if id, _ := tv.nav.SelectedID(); id != "alpha" {
		t.Fatalf("expected 'gg' to jump to the first row (alpha), got %q", id)
	}
}

// H/L scroll the viewport sideways -- capitalized since h/l (and the
// arrow keys) are already taken by collapse/expand. This is the only
// way to read a row wider than the viewport that a RowRenderer never
// word-wraps, e.g. one long, unbroken address/ID.
func TestTreeViewHorizontalScrollRevealsLongRow(t *testing.T) {
	longID := "abcdefghijklmnopqrstuvwxyz0123456789"
	tv := NewTreeView(testRenderer{})
	tv.SetTree([]Node{{ID: longID, Height: 1, Payload: testItem{longID}}})
	m, _ := tv.Update(tea.WindowSizeMsg{Width: 10, Height: 5})
	*tv = m.(TreeView)

	initial := tv.View()
	if !strings.Contains(initial, "abcdefgh") {
		t.Fatalf("expected the start of the long row visible initially, got %q", initial)
	}
	if strings.Contains(initial, "0123456789") {
		t.Fatalf("expected the tail of the long row clipped off-screen initially, got %q", initial)
	}

	for i := 0; i < 30; i++ {
		sendKey(tv, "L")
	}
	scrolled := tv.View()
	if !strings.Contains(scrolled, "0123456789") {
		t.Fatalf("expected 'L' to eventually reveal the rest of the row, got %q", scrolled)
	}

	for i := 0; i < 30; i++ {
		sendKey(tv, "H")
	}
	back := tv.View()
	if !strings.Contains(back, "abcdefgh") {
		t.Fatalf("expected 'H' to scroll back to the start, got %q", back)
	}
}

// Regression guard: 'h'/'l' (lowercase) and the arrow keys must keep
// their existing collapse/expand meaning, not be reinterpreted as
// horizontal scroll now that H/L exist.
func TestTreeViewLowercaseHLStillCollapseExpand(t *testing.T) {
	tv := newTestTreeView(40, 10)
	sendKey(tv, "l") // expand alpha (already expanded by default; no-op either way)
	if strings.Contains(tv.View(), "alpha.1") == false {
		t.Fatalf("expected alpha.1 still visible after 'l', got:\n%s", tv.View())
	}
	sendKey(tv, "h")
	if strings.Contains(tv.View(), "alpha.1") {
		t.Fatalf("expected 'h' to collapse alpha (hiding alpha.1), got:\n%s", tv.View())
	}
}
