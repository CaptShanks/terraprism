package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/CaptShanks/terraprism/internal/tfplan"
)

// These tests drive real key/mouse messages through Update() and assert
// on treeView's offset/selection -- the underlying scroll-offset math
// itself is exhaustively covered by internal/foldtree's own test suite
// (including the exact oscillation/boundary regressions these mirror),
// so these are wiring/integration checks confirming the tui package
// drives that library correctly, not re-tests of the algorithm.

func manyResourcesPlan(n int) *tfplan.Plan {
	resources := make([]tfplan.Resource, n)
	for i := range resources {
		resources[i] = tfplan.Resource{
			Address:    fmt.Sprintf("null_resource.r%d", i),
			Action:     tfplan.ActionUpdate,
			Attributes: []tfplan.Attribute{leaf("x", tfplan.ActionUpdate, tfplan.KindNumber, "1", "2")},
		}
	}
	return &tfplan.Plan{Resources: resources}
}

// selectedRowVisible reports whether the currently selected row is
// within the viewport's visible range.
func selectedRowVisible(mm Model) bool {
	nav := mm.treeView.State()
	idx := nav.SelectedIndex()
	if idx < 0 {
		return true
	}
	return nav.RowVisible(idx)
}

// selectedLineStart returns the currently selected row's starting line.
func selectedLineStart(mm Model) int {
	nav := mm.treeView.State()
	idx := nav.SelectedIndex()
	start, _ := nav.RowLineRange(idx)
	return start
}

// Regression: scrolling far away with the mouse wheel while the cursor
// sits on the first (or last) item, then pressing the boundary key that
// can't move the cursor further (k at the top, j at the bottom), used to
// nudge the viewport back by only one line per keypress. It should snap
// back immediately instead, exactly like moving the cursor does.
func TestBoundaryKeyAfterMouseScrollSnapsBackImmediately(t *testing.T) {
	tests := []struct {
		name        string
		wheelButton tea.MouseButton
		boundaryKey string
	}{
		{name: "top boundary after scrolling down", wheelButton: tea.MouseButtonWheelDown, boundaryKey: "k"},
		{name: "bottom boundary after scrolling up", wheelButton: tea.MouseButtonWheelUp, boundaryKey: "j"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := manyResourcesPlan(40)
			m := NewModel(plan, "")
			model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
			mm := model.(Model)

			if tc.boundaryKey == "j" {
				// Move the cursor to the last item first, so 'j' is the
				// boundary key (can't move further down).
				for i := 0; i < len(plan.Resources)-1; i++ {
					model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
					mm = model.(Model)
				}
			}

			for i := 0; i < 15; i++ {
				model, _ = mm.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tc.wheelButton})
				mm = model.(Model)
			}
			scrolledOffset := mm.treeView.State().Offset()
			if selectedRowVisible(mm) {
				t.Fatalf("expected the mouse wheel to scroll the cursor's row out of view, but it's still visible at Offset=%d", scrolledOffset)
			}

			model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.boundaryKey)})
			mm = model.(Model)

			topLine := mm.treeView.State().Offset()
			bottomLine := topLine + mm.treeView.Height() - 1
			line := selectedLineStart(mm)
			if line < topLine || line > bottomLine {
				t.Fatalf("cursor should be visible after a single %q press (was scrolled to %d), got viewport=[%d,%d] selectedLine=%d",
					tc.boundaryKey, scrolledOffset, topLine, bottomLine, line)
			}
		})
	}
}

// Free-scrolling past the boundary must still work when the viewport is
// already where the cursor is (no mouse drift to correct for) — this is
// the behavior the boundary branches exist for in the first place.
// Reaching the last item via 'j' leaves its row at the exact bottom edge
// of the viewport; one more 'j' at that point is the "already at cursor"
// case, not a mouse-drift case, so it should free-scroll by exactly one
// more line rather than snap.
func TestBoundaryKeyFreeScrollsWhenAlreadyAtCursor(t *testing.T) {
	plan := manyResourcesPlan(40)
	m := NewModel(plan, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	mm := model.(Model)

	for i := 0; i < len(plan.Resources)-1; i++ {
		model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		mm = model.(Model)
	}
	if !selectedRowVisible(mm) {
		t.Fatalf("cursor should already be visible after reaching the last item via keyboard nav")
	}
	before := mm.treeView.State().Offset()

	model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	mm = model.(Model)

	if got := mm.treeView.State().Offset(); got != before+1 {
		t.Fatalf("expected free-scroll by exactly one line past the last item, got Offset %d -> %d", before, got)
	}
}

// Regression: repeatedly free-scrolling past the last item used to enter
// an unstable two-value oscillation instead of settling.
func TestFreeScrollPastLastItemSettlesWithoutOscillating(t *testing.T) {
	plan := manyResourcesPlan(40)
	m := NewModel(plan, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	mm := model.(Model)

	for i := 0; i < len(plan.Resources)-1; i++ {
		model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		mm = model.(Model)
	}

	var offsets []int
	for i := 0; i < 60; i++ {
		model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		mm = model.(Model)
		offsets = append(offsets, mm.treeView.State().Offset())
		if !selectedRowVisible(mm) {
			t.Fatalf("press #%d: selection scrolled out of view (Offset=%d)", i+1, mm.treeView.State().Offset())
		}
	}

	last := offsets[len(offsets)-1]
	for i := len(offsets) - 10; i < len(offsets); i++ {
		if offsets[i] != last {
			t.Fatalf("viewport did not settle, still changing after 50+ presses: %v", offsets[len(offsets)-15:])
		}
	}
	if got := selectedLineStart(mm); got != last {
		t.Fatalf("expected the selected line to settle pinned at the top of the viewport (Offset=%d), got selectedLine=%d", last, got)
	}
}

// Reaching the last row of an expanded resource that is itself the last
// (or only) displayed resource: the selection has nowhere left to go,
// but further 'j' presses may still free-scroll the viewport to reveal
// the trailing "End of Plan" footer, exactly like being stuck at a plain
// resource row — under the unified single-cursor model there's no
// special case for "stuck deep inside a fold hierarchy" any more (that
// distinction only existed because resource-level and fold-block-level
// navigation used to be two separate cursors). It must still settle
// rather than oscillate, and the selection itself must never move once
// there's nothing left to select.
func TestLastFoldBlockOfLastResourceStopsScrollingOnceStuck(t *testing.T) {
	nested := mapBlock("values", tfplan.ActionUpdate,
		leaf("replicaCount", tfplan.ActionUpdate, tfplan.KindNumber, "2", "3"))
	metadata := mapBlock("metadata", tfplan.ActionUpdate, nested)

	resources := []tfplan.Resource{
		{Address: "helm_release.only", Action: tfplan.ActionUpdate, Attributes: withPaths([]tfplan.Attribute{metadata}, "")},
	}
	plan := &tfplan.Plan{Resources: resources}

	m := NewModel(plan, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	mm := model.(Model)

	model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyEnter}) // expand the resource
	mm = model.(Model)
	model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")}) // expand its sub-blocks
	mm = model.(Model)

	totalRows := len(mm.treeView.State().Rows())
	if totalRows <= 1 {
		t.Fatalf("expected more than just the resource row after expanding, got %d rows", totalRows)
	}

	// Step down to the last row.
	for i := 0; i < totalRows; i++ {
		model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		mm = model.(Model)
	}
	if mm.treeView.State().SelectedIndex() != totalRows-1 {
		t.Fatalf("expected selection at the last row (%d), got %d", totalRows-1, mm.treeView.State().SelectedIndex())
	}
	stuckIndex := mm.treeView.State().SelectedIndex()

	// Further 'j' presses must never move the selection — there's
	// nothing left to select — and the viewport's free-scroll (revealing
	// the trailing footer) must settle rather than oscillate.
	var offsets []int
	for i := 0; i < 30; i++ {
		model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		mm = model.(Model)
		if mm.treeView.State().SelectedIndex() != stuckIndex {
			t.Fatalf("selection should stay at the last row, got %d", mm.treeView.State().SelectedIndex())
		}
		offsets = append(offsets, mm.treeView.State().Offset())
	}
	last := offsets[len(offsets)-1]
	for i := len(offsets) - 10; i < len(offsets); i++ {
		if offsets[i] != last {
			t.Fatalf("viewport did not settle while stuck at the last row: %v", offsets[len(offsets)-15:])
		}
	}
}
