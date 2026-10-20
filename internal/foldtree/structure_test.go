package foldtree

import (
	"fmt"
	"testing"
)

// A negative height is a caller bug, not something foldtree should
// choose to trust — it must be treated as zero rather than corrupting
// the cumulative line arithmetic (which would otherwise make later
// rows start at negative lines, or totalLines go negative).
func TestNegativeHeightTreatedAsZero(t *testing.T) {
	tree := []Node{leaf("a", -5), leaf("b", 1), leaf("c", -3)}
	s := New(10)
	s.SetTree(tree)

	if s.TotalLines() < 0 {
		t.Fatalf("TotalLines = %d, must never be negative", s.TotalLines())
	}
	startA, _ := s.RowLineRange(0)
	startB, _ := s.RowLineRange(1)
	startC, _ := s.RowLineRange(2)
	if startA != 0 || startB != 0 || startC != 1 {
		t.Fatalf("negative heights should behave like zero: starts = %d,%d,%d, want 0,0,1", startA, startB, startC)
	}

	s.MoveToBottom()
	s.MoveDown() // free-scroll past the boundary; must not panic or go negative
	if s.Offset() < 0 {
		t.Fatalf("Offset went negative: %d", s.Offset())
	}
}

// A zero-height row is a degenerate but not invalid shape (e.g. a
// purely structural grouping node with nothing of its own to show) —
// navigation must still be able to select it without dividing by zero
// or producing an inverted line range.
func TestZeroHeightRowIsSelectable(t *testing.T) {
	tree := []Node{leaf("a", 0), leaf("b", 1), leaf("c", 0)}
	s := New(5)
	s.SetTree(tree)

	for i := 0; i < len(tree); i++ {
		id, ok := s.SelectedID()
		if !ok {
			t.Fatalf("step %d: expected a selection", i)
		}
		if id != tree[i].ID {
			t.Fatalf("step %d: SelectedID = %q, want %q", i, id, tree[i].ID)
		}
		start, end := s.RowLineRange(s.SelectedIndex())
		if end < start-1 { // end == start-1 is the valid "empty" row case
			t.Fatalf("step %d: inverted line range [%d,%d]", i, start, end)
		}
		s.MoveDown()
	}
}

// Duplicate IDs are a caller contract violation (IDs should be unique
// and stable), but foldtree must not panic or infinite-loop over them —
// it should behave deterministically, even if which one "wins" a
// selection-by-ID lookup is unspecified.
func TestDuplicateIDsDoNotPanic(t *testing.T) {
	tree := []Node{leaf("dup", 1), leaf("dup", 1), leaf("dup", 1)}
	s := New(5)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on duplicate IDs: %v", r)
		}
	}()

	s.SetTree(tree)
	if got := len(s.Rows()); got != 3 {
		t.Fatalf("got %d rows, want 3 (duplicate IDs must not merge rows)", got)
	}
	s.MoveToBottom()
	s.ToggleCollapse("dup")
	s.MoveToTop()
	s.MoveDown()
	s.MoveDown()
}

// A very wide fanout (tens of thousands of siblings at one level) must
// flatten and navigate correctly and promptly — no accidental
// quadratic behavior in Flatten, SetTree, or cursor movement.
func TestWideFanoutIsCorrectAndFast(t *testing.T) {
	const n = 50_000
	s := New(20)
	s.SetTree(flatRows(n, 1))

	if got := len(s.Rows()); got != n {
		t.Fatalf("got %d rows, want %d", got, n)
	}

	s.MoveToBottom()
	id, _ := s.SelectedID()
	if want := fmt.Sprintf("r%d", n-1); id != want {
		t.Fatalf("SelectedID after MoveToBottom = %q, want %q", id, want)
	}
	assertVisible(t, s)

	s.MoveToTop()
	id, _ = s.SelectedID()
	if id != "r0" {
		t.Fatalf("SelectedID after MoveToTop = %q, want r0", id)
	}
}

// An extremely deep single-child chain (far deeper than any real
// attribute tree) must not overflow the stack anywhere in the
// navigation path — SetTree, CollapseAll, and repeated MoveDown all
// walk the tree in some form.
func TestExtremelyDeepChainNavigatesWithoutOverflow(t *testing.T) {
	const depth = 100_000
	root := chainOf(depth)
	s := New(10)
	s.SetTree([]Node{root})

	if got := len(s.Rows()); got != depth {
		t.Fatalf("got %d rows, want %d", got, depth)
	}

	s.CollapseAll() // collapses every node in the chain except the innermost leaf
	if got := len(s.Rows()); got != 1 {
		t.Fatalf("after CollapseAll on a chain, got %d rows, want 1 (only the outermost root)", got)
	}

	s.ExpandAll()
	for i := 0; i < 100; i++ {
		s.MoveDown()
	}
	assertVisible(t, s)
}

// Replacing the tree with one that doesn't contain the previously
// selected row at all (not even under a different collapse state) must
// fall back to a valid, in-range selection rather than a stale or
// negative index.
func TestSetTreeWithCompletelyDisjointTreeFallsBackCleanly(t *testing.T) {
	s := New(10)
	s.SetTree([]Node{block("x", 1, leaf("x.1", 1), leaf("x.2", 1))})
	s.MoveDown() // select x.1

	s.SetTree([]Node{leaf("unrelated-a", 1), leaf("unrelated-b", 1)})

	idx := s.SelectedIndex()
	if idx < 0 || idx >= 2 {
		t.Fatalf("SelectedIndex = %d out of range after a fully disjoint SetTree", idx)
	}
	assertVisible(t, s)
}

// Many siblings each independently collapsed/expanded in an interleaved
// pattern, then jumping to the extremes, must still produce a row count
// matching exactly the structurally-visible nodes (no drift between the
// collapse map and the flattened rows).
func TestInterleavedCollapseThenJumpToExtremes(t *testing.T) {
	const n = 200
	nodes := make([]Node, n)
	for i := range nodes {
		id := fmt.Sprintf("b%d", i)
		nodes[i] = block(id, 1, leaf(id+".child", 1))
	}
	s := New(15)
	s.SetTree(nodes)

	wantRows := 2 * n
	for i := 0; i < n; i += 2 {
		s.ToggleCollapse(fmt.Sprintf("b%d", i))
		wantRows-- // each collapsed block hides exactly its one child
	}
	if got := len(s.Rows()); got != wantRows {
		t.Fatalf("got %d rows after interleaved collapse, want %d", got, wantRows)
	}

	s.MoveToBottom()
	assertVisible(t, s)
	lastID, _ := s.SelectedID()
	if lastID != fmt.Sprintf("b%d", n-1) && lastID != fmt.Sprintf("b%d.child", n-1) {
		t.Fatalf("unexpected last row ID: %q", lastID)
	}

	s.MoveToTop()
	assertVisible(t, s)
	if id, _ := s.SelectedID(); id != "b0" {
		t.Fatalf("SelectedID at top = %q, want b0", id)
	}
}
