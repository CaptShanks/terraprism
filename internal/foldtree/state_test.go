package foldtree

import (
	"strings"
	"testing"
)

// assertVisible fails the test unless the selected row's start line is
// currently within the viewport's visible range — the core invariant
// every cursor-moving operation must restore before returning.
func assertVisible(t *testing.T, s *State) {
	t.Helper()
	idx := s.SelectedIndex()
	if idx < 0 {
		return
	}
	start, _ := s.RowLineRange(idx)
	top, bottom := s.VisibleRange()
	if start < top || start > bottom {
		t.Fatalf("selected row %d (start=%d) not visible in [%d,%d]", idx, start, top, bottom)
	}
}

func TestMoveDownAndUpThroughFlatList(t *testing.T) {
	s := New(5)
	s.SetTree(flatRows(20, 1))

	for i := 0; i < 10; i++ {
		s.MoveDown()
		if got, want := s.SelectedIndex(), i+1; got != want {
			t.Fatalf("after %d MoveDown calls: SelectedIndex = %d, want %d", i+1, got, want)
		}
		assertVisible(t, s)
	}
	for i := 0; i < 5; i++ {
		s.MoveUp()
		assertVisible(t, s)
	}
	if got, want := s.SelectedIndex(), 5; got != want {
		t.Fatalf("SelectedIndex = %d, want %d", got, want)
	}
}

func TestMoveToTopAndBottom(t *testing.T) {
	s := New(5)
	s.SetTree(flatRows(50, 1))

	s.MoveToBottom()
	if got, want := s.SelectedIndex(), 49; got != want {
		t.Fatalf("MoveToBottom: SelectedIndex = %d, want %d", got, want)
	}
	assertVisible(t, s)

	s.MoveToTop()
	if got, want := s.SelectedIndex(), 0; got != want {
		t.Fatalf("MoveToTop: SelectedIndex = %d, want %d", got, want)
	}
	if s.Offset() != 0 {
		t.Fatalf("MoveToTop: Offset = %d, want 0", s.Offset())
	}
}

func TestSelectIndexJumpsDirectlyAndClamps(t *testing.T) {
	s := New(5)
	s.SetTree(flatRows(50, 1))

	s.SelectIndex(30)
	if got, want := s.SelectedIndex(), 30; got != want {
		t.Fatalf("SelectIndex(30): SelectedIndex = %d, want %d", got, want)
	}
	assertVisible(t, s)

	s.SelectIndex(-5)
	if got, want := s.SelectedIndex(), 0; got != want {
		t.Fatalf("SelectIndex(-5): SelectedIndex = %d, want %d (clamped)", got, want)
	}

	s.SelectIndex(999)
	if got, want := s.SelectedIndex(), 49; got != want {
		t.Fatalf("SelectIndex(999): SelectedIndex = %d, want %d (clamped)", got, want)
	}
	assertVisible(t, s)
}

func TestSelectIndexOnEmptyTreeIsSafe(t *testing.T) {
	s := New(5)
	s.SetTree(nil)
	s.SelectIndex(3) // must not panic
	if _, ok := s.SelectedID(); ok {
		t.Fatalf("expected no selection on an empty tree")
	}
}

func TestPageDownAndPageUp(t *testing.T) {
	s := New(10)
	s.SetTree(flatRows(100, 1))

	s.PageDown()
	if s.SelectedIndex() <= 0 {
		t.Fatalf("PageDown should move forward, got SelectedIndex = %d", s.SelectedIndex())
	}
	assertVisible(t, s)
	afterFirstPage := s.SelectedIndex()

	s.PageDown()
	if s.SelectedIndex() <= afterFirstPage {
		t.Fatalf("second PageDown should move further forward, got %d -> %d", afterFirstPage, s.SelectedIndex())
	}
	assertVisible(t, s)

	beforePageUp := s.SelectedIndex()
	s.PageUp()
	if s.SelectedIndex() >= beforePageUp {
		t.Fatalf("PageUp should move backward from %d, got %d", beforePageUp, s.SelectedIndex())
	}
	assertVisible(t, s)
}

func TestMoveMouseDoesNotMoveCursor(t *testing.T) {
	s := New(5)
	s.SetTree(flatRows(50, 1))
	s.MoveDown()
	s.MoveDown()
	before := s.SelectedIndex()

	s.MoveMouse(20)

	if s.SelectedIndex() != before {
		t.Fatalf("MoveMouse changed SelectedIndex from %d to %d", before, s.SelectedIndex())
	}
}

func TestToggleCollapseHidesAndShowsChildren(t *testing.T) {
	tree := []Node{block("a", 1, leaf("a.1", 1), leaf("a.2", 1)), leaf("b", 1)}
	s := New(10)
	s.SetTree(tree)

	if len(s.Rows()) != 4 {
		t.Fatalf("expanded: got %d rows, want 4", len(s.Rows()))
	}

	s.ToggleCollapse("a")
	if len(s.Rows()) != 2 {
		t.Fatalf("collapsed: got %d rows, want 2: %+v", len(s.Rows()), s.Rows())
	}

	s.ToggleCollapse("a")
	if len(s.Rows()) != 4 {
		t.Fatalf("re-expanded: got %d rows, want 4", len(s.Rows()))
	}
}

// Collapsing an ancestor of the currently selected row must move the
// selection to the nearest still-visible ancestor rather than leaving
// it dangling on a row that no longer exists.
func TestCollapsingAncestorOfSelectionMovesSelectionUp(t *testing.T) {
	tree := []Node{block("a", 1, block("a.1", 1, leaf("a.1.1", 1)))}
	s := New(10)
	s.SetTree(tree)

	s.MoveDown() // a.1
	s.MoveDown() // a.1.1
	id, _ := s.SelectedID()
	if id != "a.1.1" {
		t.Fatalf("setup: SelectedID = %q, want a.1.1", id)
	}

	s.ToggleCollapse("a") // hides both a.1 and a.1.1

	id, ok := s.SelectedID()
	if !ok {
		t.Fatalf("expected a selection to survive collapsing an ancestor")
	}
	if id != "a" {
		t.Fatalf("SelectedID after collapsing ancestor = %q, want %q (nearest visible ancestor)", id, "a")
	}
	assertVisible(t, s)
}

func TestCollapseAllAndExpandAll(t *testing.T) {
	tree := []Node{
		block("a", 1, leaf("a.1", 1)),
		block("b", 1, block("b.1", 1, leaf("b.1.1", 1))),
	}
	s := New(10)
	s.SetTree(tree)

	s.CollapseAll()
	if got, want := len(s.Rows()), 2; got != want {
		t.Fatalf("after CollapseAll: got %d rows, want %d: %+v", got, want, s.Rows())
	}

	s.ExpandAll()
	if got, want := len(s.Rows()), countNodes(tree); got != want {
		t.Fatalf("after ExpandAll: got %d rows, want %d (all nodes)", got, want)
	}
}

// A collapsible node with zero children has nothing to hide; toggling
// it must be a harmless no-op, not a panic or a phantom row change.
func TestToggleCollapseOnChildlessNodeIsNoop(t *testing.T) {
	tree := []Node{{ID: "a", Height: 1, Collapsible: true}}
	s := New(10)
	s.SetTree(tree)

	before := len(s.Rows())
	s.ToggleCollapse("a")
	if len(s.Rows()) != before {
		t.Fatalf("toggling a childless node changed row count: %d -> %d", before, len(s.Rows()))
	}
}

func TestSetTreeReplacesTreeAndClampsStaleSelection(t *testing.T) {
	s := New(10)
	s.SetTree(flatRows(10, 1))
	s.MoveToBottom()

	s.SetTree(flatRows(3, 1))

	if s.SelectedIndex() < 0 || s.SelectedIndex() >= 3 {
		t.Fatalf("SelectedIndex = %d out of range after shrinking tree to 3 rows", s.SelectedIndex())
	}
	assertVisible(t, s)
}

func threeLevelTree() []Node {
	return []Node{
		block("a", 1, block("a.1", 1, leaf("a.1.1", 1))),
		block("b", 1, block("b.1", 1, leaf("b.1.1", 1))),
	}
}

// ExpandSubtree/CollapseSubtree must affect only the target node's own
// branch, leaving sibling branches elsewhere in the tree untouched —
// the opposite of SetFoldLevel/ExpandAll/CollapseAll, which are
// deliberately tree-wide.
func TestExpandAndCollapseSubtreeAreScopedToOneBranch(t *testing.T) {
	s := New(20)
	s.SetTree(threeLevelTree())

	s.CollapseAll()
	if got, want := len(s.Rows()), 2; got != want {
		t.Fatalf("setup: got %d rows after CollapseAll, want %d", got, want)
	}

	s.ExpandSubtree("a")
	if got, want := len(s.Rows()), 4; got != want { // a, a.1, a.1.1, b (b's subtree still collapsed)
		t.Fatalf("after ExpandSubtree(a): got %d rows, want %d: %+v", got, want, s.Rows())
	}
	for _, row := range s.Rows() {
		if strings.HasPrefix(row.ID, "b") && row.ID != "b" {
			t.Fatalf("ExpandSubtree(a) leaked into b's branch: row %q is visible", row.ID)
		}
	}

	s.ExpandAll()
	s.CollapseSubtree("a")
	if got, want := len(s.Rows()), 4; got != want { // a (collapsed), b, b.1, b.1.1
		t.Fatalf("after CollapseSubtree(a): got %d rows, want %d: %+v", got, want, s.Rows())
	}
	if !s.IsCollapsed("a") {
		t.Fatalf("expected 'a' itself to be collapsed after CollapseSubtree(a)")
	}
	if s.IsCollapsed("b") {
		t.Fatalf("CollapseSubtree(a) should not affect 'b'")
	}
}

func TestExpandCollapseSubtreeUnknownIDIsNoop(t *testing.T) {
	s := New(20)
	s.SetTree(threeLevelTree())
	before := len(s.Rows())

	s.ExpandSubtree("does-not-exist")
	s.CollapseSubtree("does-not-exist")

	if got := len(s.Rows()); got != before {
		t.Fatalf("subtree op on an unknown ID changed row count: %d -> %d", before, got)
	}
}

func TestSetFoldLevelCollapsesByDepth(t *testing.T) {
	s := New(20)
	s.SetTree(threeLevelTree())

	s.SetFoldLevel(0)
	if got, want := len(s.Rows()), 2; got != want { // just the two depth-0 roots
		t.Fatalf("level 0: got %d rows, want %d: %+v", got, want, s.Rows())
	}

	s.SetFoldLevel(1)
	if got, want := len(s.Rows()), 4; got != want { // roots + their depth-1 children
		t.Fatalf("level 1: got %d rows, want %d: %+v", got, want, s.Rows())
	}

	s.SetFoldLevel(2)
	if got, want := len(s.Rows()), countNodes(threeLevelTree()); got != want {
		t.Fatalf("level 2 (>= max depth): got %d rows, want %d (fully expanded)", got, want)
	}

	s.SetFoldLevel(-5) // negative must clamp to 0, not panic or go out of range
	if got, want := len(s.Rows()), 2; got != want {
		t.Fatalf("negative level: got %d rows, want %d", got, want)
	}
}

// SetFoldLevel is a tree-wide operation: it must apply the same way
// regardless of where the cursor currently sits, unlike an operation
// scoped to the current selection's subtree.
func TestSetFoldLevelIsGlobalRegardlessOfCursor(t *testing.T) {
	s := New(20)
	s.SetTree(threeLevelTree())

	s.MoveDown() // move into "a"'s subtree
	s.MoveDown()
	id, _ := s.SelectedID()
	if id == "a" {
		t.Fatalf("setup: expected cursor to have moved into a's subtree, still at %q", id)
	}

	s.SetFoldLevel(0)
	if got, want := len(s.Rows()), 2; got != want {
		t.Fatalf("SetFoldLevel(0) from a non-root cursor position: got %d rows, want %d (both branches collapsed)", got, want)
	}
}

func TestSetFoldLevelSurvivesDeepChain(t *testing.T) {
	const depth = 50_000
	s := New(10)
	s.SetTree([]Node{chainOf(depth)})

	const level = 100
	s.SetFoldLevel(level)

	if got, want := len(s.Rows()), level+1; got != want {
		t.Fatalf("got %d rows, want %d (depths 0..level)", got, want)
	}
}

func TestIsCollapsedReflectsSetCollapsed(t *testing.T) {
	s := New(10)
	s.SetTree([]Node{block("a", 1, leaf("a.1", 1))})

	if s.IsCollapsed("a") {
		t.Fatalf("expected 'a' to start expanded")
	}
	s.SetCollapsed("a", true)
	if !s.IsCollapsed("a") {
		t.Fatalf("expected 'a' to report collapsed after SetCollapsed(true)")
	}
	s.SetCollapsed("a", false)
	if s.IsCollapsed("a") {
		t.Fatalf("expected 'a' to report expanded after SetCollapsed(false)")
	}
}

func TestNewClampsNegativeHeight(t *testing.T) {
	s := New(-5)
	s.SetTree(flatRows(3, 1))
	if top, bottom := s.VisibleRange(); bottom < top {
		t.Fatalf("VisibleRange inverted for negative-height viewport: [%d,%d]", top, bottom)
	}
}

func TestSelectParentWalksUpwardThenStops(t *testing.T) {
	s := New(20)
	s.SetTree(threeLevelTree())

	s.MoveDown() // a.1
	s.MoveDown() // a.1.1
	if id, _ := s.SelectedID(); id != "a.1.1" {
		t.Fatalf("setup: SelectedID = %q, want a.1.1", id)
	}

	s.SelectParent()
	if id, _ := s.SelectedID(); id != "a.1" {
		t.Fatalf("after 1st SelectParent: SelectedID = %q, want a.1", id)
	}

	s.SelectParent()
	if id, _ := s.SelectedID(); id != "a" {
		t.Fatalf("after 2nd SelectParent: SelectedID = %q, want a", id)
	}

	s.SelectParent() // already at a root: no-op
	if id, _ := s.SelectedID(); id != "a" {
		t.Fatalf("SelectParent on a root row changed selection to %q, want it to stay at a", id)
	}
}

func TestSelectParentIsSafeOnEmptyTree(t *testing.T) {
	s := New(10)
	s.SetTree(nil)
	s.SelectParent() // must not panic
	if _, ok := s.SelectedID(); ok {
		t.Fatalf("expected no selection")
	}
}

func TestEmptyTreeIsSafe(t *testing.T) {
	s := New(10)
	s.SetTree(nil)

	if _, ok := s.SelectedID(); ok {
		t.Fatalf("expected no selection on an empty tree")
	}
	if idx := s.SelectedIndex(); idx != -1 {
		t.Fatalf("SelectedIndex = %d, want -1", idx)
	}

	// None of these should panic.
	s.MoveUp()
	s.MoveDown()
	s.PageUp()
	s.PageDown()
	s.MoveToTop()
	s.MoveToBottom()
	s.MoveMouse(5)
	s.ToggleCollapse("nonexistent")
}
