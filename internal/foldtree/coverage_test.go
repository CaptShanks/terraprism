package foldtree

import "testing"

// These tests target defensive branches the behavioral test suite
// doesn't naturally reach, but that are still reachable through the
// public API with an input a real caller could plausibly pass (a
// negative height from a terminal resize race, a stale or bad index
// computed elsewhere and handed to RowLineRange/RowVisible).
//
// Deliberately not included here: forcing State.cursor out of range by
// direct field assignment to cover clampCursor/clampedCursor's other
// branches, or calling rowAtLine directly on an empty State. Every
// public mutating method keeps the cursor in [0, len(rows)) on its own,
// and rowAtLine's only caller (pageBy) already guards against empty
// rows before calling it — so those branches are unreachable through
// real usage, and a test that only reaches them via direct, same-package
// field manipulation would inflate coverage without adding confidence
// about anything a real caller could trigger.

func TestPageDownAndPageUpFallBackToMoveWhenHeightIsZero(t *testing.T) {
	s := New(0)
	s.SetTree(flatRows(10, 1))
	if s.SelectedIndex() != 0 {
		t.Fatalf("setup: SelectedIndex = %d, want 0", s.SelectedIndex())
	}

	s.PageDown()
	if got, want := s.SelectedIndex(), 1; got != want {
		t.Fatalf("PageDown with height=0: SelectedIndex = %d, want %d (should fall back to MoveDown)", got, want)
	}

	s.PageUp()
	if got, want := s.SelectedIndex(), 0; got != want {
		t.Fatalf("PageUp with height=0: SelectedIndex = %d, want %d (should fall back to MoveUp)", got, want)
	}
}

func TestRowStartVisibleRejectsNegativeIndex(t *testing.T) {
	s := New(10)
	s.SetTree(flatRows(5, 1))
	if s.rowStartVisible(-1) {
		t.Fatalf("rowStartVisible(-1) = true, want false")
	}
}

func TestRowLineRangeRejectsNegativeIndex(t *testing.T) {
	s := New(10)
	s.SetTree(flatRows(5, 1))
	start, end := s.RowLineRange(-1)
	if start != 0 || end != -1 {
		t.Fatalf("RowLineRange(-1) = (%d,%d), want (0,-1)", start, end)
	}
}

func TestSetHeightClampsNegative(t *testing.T) {
	s := New(10)
	s.SetTree(flatRows(5, 1))
	s.SetHeight(-3)
	if top, bottom := s.VisibleRange(); bottom < top {
		t.Fatalf("VisibleRange inverted after SetHeight(-3): [%d,%d]", top, bottom)
	}
	// A height that clamped to 0 means at most one line is ever visible.
	if top, bottom := s.VisibleRange(); bottom-top > 0 {
		t.Fatalf("expected a clamped-to-zero height to produce an empty visible range, got [%d,%d]", top, bottom)
	}
}

func TestSetExtraPaddingClampsNegative(t *testing.T) {
	s := New(10)
	s.SetTree(flatRows(5, 1))
	s.SetExtraPadding(-100)
	if s.TotalLines() != 5 {
		t.Fatalf("negative SetExtraPadding should clamp to 0: TotalLines = %d, want 5", s.TotalLines())
	}
}
