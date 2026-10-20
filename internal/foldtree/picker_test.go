package foldtree

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "up", "down", "esc", "enter":
		var t tea.KeyType
		switch s {
		case "up":
			t = tea.KeyUp
		case "down":
			t = tea.KeyDown
		case "esc":
			t = tea.KeyEsc
		case "enter":
			t = tea.KeyEnter
		}
		return tea.KeyMsg{Type: t}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestPickerSingleSelectCursorMovementAndClamping(t *testing.T) {
	p := NewPicker([]string{"a", "b", "c"}, func(s string) string { return s })

	if got := p.Update(key("up")); got != PickerNone {
		t.Fatalf("up at top: got %v, want PickerNone", got)
	}
	if p.Cursor() != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", p.Cursor())
	}

	p.Update(key("down"))
	p.Update(key("down"))
	if p.Cursor() != 2 {
		t.Fatalf("cursor = %d, want 2", p.Cursor())
	}
	p.Update(key("down"))
	if p.Cursor() != 2 {
		t.Fatalf("cursor should clamp at last index 2, got %d", p.Cursor())
	}
}

func TestPickerSingleSelectEnterAndSpaceBothApply(t *testing.T) {
	for _, k := range []string{"enter", " "} {
		p := NewPicker([]string{"a", "b"}, func(s string) string { return s })
		p.Update(key("down"))
		if got := p.Update(key(k)); got != PickerApply {
			t.Fatalf("key %q: got %v, want PickerApply", k, got)
		}
		if p.Highlighted() != "b" {
			t.Fatalf("Highlighted() = %q, want %q", p.Highlighted(), "b")
		}
	}
}

func TestPickerSingleSelectSetCurrentSeedsCursorAndMarker(t *testing.T) {
	p := NewPicker([]string{"a", "b", "c"}, func(s string) string { return s })
	p.SetCurrent("c")
	if p.Cursor() != 2 {
		t.Fatalf("SetCurrent should move cursor to the match, got %d", p.Cursor())
	}
	if p.Current() != "c" {
		t.Fatalf("Current() = %q, want %q", p.Current(), "c")
	}
	// Moving the cursor away must not change what SetCurrent marked as
	// active -- the marker (Current) and the cursor are independent, the
	// same way today's sort picker shows "*" on the active order even
	// while the cursor highlight sits on a different row being browsed.
	p.Update(key("up"))
	if p.Current() != "c" {
		t.Fatalf("Current() changed after moving cursor: got %q, want %q", p.Current(), "c")
	}
	view := p.View()
	if !strings.Contains(view, "●") {
		t.Fatalf("View() should render a marker for the current option:\n%s", view)
	}
}

func TestPickerMultiSelectToggleSelectAllClearAll(t *testing.T) {
	p := NewPicker([]string{"a", "b", "c"}, func(s string) string { return s })
	p.Multi = true

	if got := p.Update(key(" ")); got != PickerNone {
		t.Fatalf("space: got %v, want PickerNone", got)
	}
	if !p.Selected["a"] {
		t.Fatalf("expected 'a' selected after toggling at cursor 0")
	}
	p.Update(key(" ")) // toggle back off
	if p.Selected["a"] {
		t.Fatalf("expected 'a' unselected after toggling twice")
	}

	p.Update(key("a"))
	for _, opt := range p.Options {
		if !p.Selected[opt] {
			t.Fatalf("select-all should select every option, %q missing", opt)
		}
	}

	p.Update(key("c"))
	for _, opt := range p.Options {
		if p.Selected[opt] {
			t.Fatalf("clear-all should deselect every option, %q still set", opt)
		}
	}
}

func TestPickerMultiSelectEnterAppliesWithoutTogglingCurrentRow(t *testing.T) {
	p := NewPicker([]string{"a", "b"}, func(s string) string { return s })
	p.Multi = true
	p.Selected["a"] = true

	if got := p.Update(key("enter")); got != PickerApply {
		t.Fatalf("got %v, want PickerApply", got)
	}
	if !p.Selected["a"] || p.Selected["b"] {
		t.Fatalf("Enter must not toggle anything, got Selected=%v", p.Selected)
	}
}

func TestPickerEscAlwaysCancels(t *testing.T) {
	for _, multi := range []bool{false, true} {
		p := NewPicker([]string{"a", "b"}, func(s string) string { return s })
		p.Multi = multi
		if got := p.Update(key("esc")); got != PickerCancel {
			t.Fatalf("Multi=%v: got %v, want PickerCancel", multi, got)
		}
	}
}

func TestPickerViewRendersLabelsHintsAndCheckboxes(t *testing.T) {
	p := NewPicker([]string{"a", "b"}, func(s string) string { return "Label-" + s })
	p.Hint = func(s string) string { return "(hint-" + s + ")" }
	p.Multi = true
	p.Selected["b"] = true

	view := p.View()
	if !strings.Contains(view, "Label-a") || !strings.Contains(view, "Label-b") {
		t.Fatalf("expected both labels rendered:\n%s", view)
	}
	if !strings.Contains(view, "(hint-a)") {
		t.Fatalf("expected hint text rendered:\n%s", view)
	}
	if !strings.Contains(view, "[x]") || !strings.Contains(view, "[ ]") {
		t.Fatalf("expected one checked and one unchecked checkbox:\n%s", view)
	}
}

func TestPickerEmptyOptionsIsSafe(t *testing.T) {
	p := NewPicker([]string{}, func(s string) string { return s })
	p.Multi = true
	for _, k := range []string{"up", "down", " ", "a", "c", "enter", "esc"} {
		p.Update(key(k))
	}
	if p.View() != "" {
		t.Fatalf("expected empty view for zero options, got %q", p.View())
	}
}

// Regression test: single-select Update returns PickerApply
// unconditionally on Enter/Space, with no empty-Options guard -- a
// caller that then reads Highlighted() to get the confirmed choice
// would panic (index out of range) on a picker dynamically filtered
// down to zero options. Highlighted() must return the zero value
// instead, matching the same guard Update already applies elsewhere.
func TestPickerHighlightedIsSafeOnEmptyOptions(t *testing.T) {
	p := NewPicker([]string{}, func(s string) string { return s })
	if got := p.Highlighted(); got != "" {
		t.Fatalf("Highlighted() on empty picker = %q, want zero value", got)
	}

	if action := p.Update(key("enter")); action != PickerApply {
		t.Fatalf("Update(enter) on empty picker = %v, want PickerApply", action)
	}
	if got := p.Highlighted(); got != "" {
		t.Fatalf("Highlighted() after PickerApply on empty picker = %q, want zero value", got)
	}
}
