package foldtree

import (
	"fmt"
	"testing"
)

// TestFreeScrollSettlesWithoutOscillating is the generalized regression
// test for a real bug found in terraprism's hand-rolled navigation: once
// the viewport's top line advanced to exactly match the selected row's
// start line, the "still visible -> scroll one more" branch pushed one
// line past it, making it invisible on the very next call, which
// snapped straight back to the offset it had just left — an unstable
// two-value oscillation instead of a settled state.
//
// It's run across a range of row counts, row heights, and viewport
// heights, since the original bug only reproduced at one specific
// combination of the three against a real Terraform plan.
func TestFreeScrollSettlesWithoutOscillating(t *testing.T) {
	cases := []struct {
		rows, rowHeight, viewportHeight int
	}{
		{rows: 1, rowHeight: 1, viewportHeight: 5},
		{rows: 9, rowHeight: 1, viewportHeight: 38}, // shape of the real reproduction
		{rows: 40, rowHeight: 1, viewportHeight: 20},
		{rows: 5, rowHeight: 3, viewportHeight: 7},
		{rows: 3, rowHeight: 10, viewportHeight: 4}, // rows taller than the viewport
		{rows: 2, rowHeight: 1, viewportHeight: 1},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("rows=%d height=%d viewport=%d", tc.rows, tc.rowHeight, tc.viewportHeight), func(t *testing.T) {
			s := New(tc.viewportHeight)
			s.SetTree(flatRows(tc.rows, tc.rowHeight))
			s.MoveToBottom()

			var offsets []int
			for i := 0; i < 200; i++ {
				s.MoveDown()
				offsets = append(offsets, s.Offset())
				if !s.RowVisible(s.SelectedIndex()) {
					t.Fatalf("press #%d: selection scrolled out of view (offset=%d)", i+1, s.Offset())
				}
			}

			last := offsets[len(offsets)-1]
			settleFrom := len(offsets) - 20
			for i := settleFrom; i < len(offsets); i++ {
				if offsets[i] != last {
					t.Fatalf("did not settle, still changing after 180 presses: %v", offsets[settleFrom:])
				}
			}
		})
	}
}

// The symmetric case at the top of the list, scrolling upward.
func TestFreeScrollUpSettlesWithoutOscillating(t *testing.T) {
	s := New(20)
	s.SetTree(flatRows(40, 1))
	s.MoveToTop()

	var offsets []int
	for i := 0; i < 100; i++ {
		s.MoveUp()
		offsets = append(offsets, s.Offset())
	}
	last := offsets[len(offsets)-1]
	for i := len(offsets) - 10; i < len(offsets); i++ {
		if offsets[i] != last {
			t.Fatalf("did not settle scrolling up: %v", offsets[len(offsets)-15:])
		}
	}
}

// If the viewport was scrolled far away (mouse wheel) while the cursor
// sits on a row that can't move any further, the very next boundary
// move must snap straight back in one step, not creep back one line at
// a time and not oscillate once it arrives.
func TestBoundaryMoveAfterFarMouseScrollSnapsInOneStep(t *testing.T) {
	s := New(20)
	s.SetTree(flatRows(40, 1))
	s.MoveToBottom()

	s.MoveMouse(-1000) // scroll far up, away from the selection
	if s.RowVisible(s.SelectedIndex()) {
		t.Fatalf("expected the mouse scroll to move the selection out of view")
	}

	s.MoveDown() // boundary key: cursor can't move further

	if !s.RowVisible(s.SelectedIndex()) {
		t.Fatalf("expected selection visible after a single boundary move, offset=%d", s.Offset())
	}

	// And it must not then start oscillating on subsequent presses.
	afterSnap := s.Offset()
	for i := 0; i < 10; i++ {
		s.MoveDown()
		if s.Offset() != afterSnap {
			t.Fatalf("oscillated after snapping back: offset changed from %d to %d on press %d", afterSnap, s.Offset(), i+1)
		}
	}
}

// Extra scrollable content beyond the last row (e.g. trailing chrome
// text) must still be reachable by free-scrolling past the last row: as
// the boundary key keeps being pressed, the selected row should travel
// from wherever it was pinned (typically the bottom edge, reached via
// ordinary downward navigation) up toward the top edge, revealing
// padding underneath it one line at a time, then settle there — it must
// not gate on the total padding amount, and must not oscillate once
// settled at the top edge.
func TestFreeScrollRevealsExtraPaddingThenSettles(t *testing.T) {
	s := New(10)
	s.SetTree(flatRows(15, 1)) // more rows than the viewport height...
	s.SetExtraPadding(30)      // ...plus trailing padding beyond them.

	for i := 0; i < 14; i++ {
		s.MoveDown()
	}
	lastRowStart, _ := s.RowLineRange(s.SelectedIndex())
	pinnedAtBottom := lastRowStart - s.Offset()
	if pinnedAtBottom != 9 { // viewport height (10) - 1
		t.Fatalf("setup: expected the last row pinned at the bottom edge, got row-start-minus-offset=%d", pinnedAtBottom)
	}

	var offsets []int
	for i := 0; i < 30; i++ {
		s.MoveDown()
		offsets = append(offsets, s.Offset())
	}

	// The row should have travelled up to sit exactly at the top edge
	// (offset == its own start line) — revealing 9 lines of padding
	// underneath it along the way — and gone no further.
	if got, want := offsets[len(offsets)-1], lastRowStart; got != want {
		t.Fatalf("expected free-scroll to settle with the row pinned at the top (offset=%d), got %d", want, got)
	}
	last := offsets[len(offsets)-1]
	for i := len(offsets) - 10; i < len(offsets); i++ {
		if offsets[i] != last {
			t.Fatalf("did not settle after revealing padding: %v", offsets[len(offsets)-15:])
		}
	}
}
