package foldtree

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PickerAction reports what a keypress did to a Picker, so a caller
// embedding it can decide whether to close the overlay and what to do
// with the current selection.
type PickerAction int

const (
	// PickerNone means the keypress only moved the cursor or toggled a
	// selection; the picker should stay open.
	PickerNone PickerAction = iota
	// PickerApply means the user confirmed their choice (Enter, or Space
	// in single-select mode) — the caller should read Selected/Highlighted
	// and close the overlay.
	PickerApply
	// PickerCancel means the user backed out (Esc) — the caller decides
	// what, if anything, to reset before closing the overlay.
	PickerCancel
)

// Picker is a single- or multi-select list overlay. It knows nothing about
// what an option represents: callers supply the option list plus a label
// (and optional hint) function, and read the selection back afterward.
//
// In single-select mode (Multi == false), Enter or Space confirms the
// option under the cursor. In multi-select mode (Multi == true), Space
// toggles the option under the cursor in Selected, 'a' selects every
// option, 'c' clears every option, and Enter confirms without changing
// the current toggles.
type Picker[T comparable] struct {
	Options []T
	Label   func(T) string
	Hint    func(T) string // optional; nil suppresses hint text
	Multi   bool

	// Selected holds multi-select state (Multi == true). Callers own it
	// directly — pre-populate it before first use, read it after
	// PickerApply/PickerCancel.
	Selected map[T]bool

	cursor  int
	current T // single-select mode: the option marked as currently active, independent of cursor position
}

// NewPicker constructs a Picker over options, using label to render each
// option's display text.
func NewPicker[T comparable](options []T, label func(T) string) *Picker[T] {
	return &Picker[T]{
		Options:  options,
		Label:    label,
		Selected: make(map[T]bool),
	}
}

// SetCurrent marks v as the currently active single-select choice (shown
// with a marker independent of cursor position) and moves the cursor to
// it, if present among Options — used to open a single-select picker with
// the cursor already on today's active choice.
func (p *Picker[T]) SetCurrent(v T) {
	p.current = v
	for i, opt := range p.Options {
		if opt == v {
			p.cursor = i
			return
		}
	}
}

// Current returns the option last marked active via SetCurrent — the
// single-select "currently applied" value, independent of the cursor.
func (p *Picker[T]) Current() T { return p.current }

// Highlighted returns the option currently under the cursor — the
// single-select "current choice" once PickerApply is returned. Returns
// the zero value of T if there are no options (e.g. a dynamically
// populated picker filtered down to nothing) rather than panicking,
// matching the same empty-Options guard Update already applies.
func (p *Picker[T]) Highlighted() T {
	if len(p.Options) == 0 {
		var zero T
		return zero
	}
	return p.Options[p.cursor]
}

// Cursor returns the current cursor index into Options.
func (p *Picker[T]) Cursor() int { return p.cursor }

// SetCursor moves the cursor to index i, clamped into range.
func (p *Picker[T]) SetCursor(i int) {
	if i < 0 {
		i = 0
	}
	if i > len(p.Options)-1 {
		i = len(p.Options) - 1
	}
	if i < 0 {
		i = 0
	}
	p.cursor = i
}

// Update handles one keypress, mutating cursor/Selected in place, and
// reports what happened.
func (p *Picker[T]) Update(msg tea.KeyMsg) PickerAction {
	switch msg.String() {
	case "esc":
		return PickerCancel

	case "enter":
		return PickerApply

	case " ":
		if p.Multi {
			if len(p.Options) > 0 {
				opt := p.Options[p.cursor]
				p.Selected[opt] = !p.Selected[opt]
			}
			return PickerNone
		}
		return PickerApply

	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
		}
		return PickerNone

	case "down", "j":
		if p.cursor < len(p.Options)-1 {
			p.cursor++
		}
		return PickerNone

	case "a":
		if p.Multi {
			for _, opt := range p.Options {
				p.Selected[opt] = true
			}
		}
		return PickerNone

	case "c":
		if p.Multi {
			for opt := range p.Selected {
				delete(p.Selected, opt)
			}
		}
		return PickerNone
	}

	return PickerNone
}

// View renders just the option rows (checkbox/marker + label + hint,
// with the cursor row highlighted) — callers wrap this with their own
// header/footer text, exactly as a caller-specific title and key-hint
// line would surround any other overlay.
func (p Picker[T]) View() string {
	var b strings.Builder
	for i, opt := range p.Options {
		marker := "  "
		if p.Multi {
			if p.Selected[opt] {
				marker = "[x]"
			} else {
				marker = "[ ]"
			}
		} else if opt == p.current {
			marker = "● "
		}

		line := marker + " " + p.Label(opt)
		if p.Hint != nil {
			if hint := p.Hint(opt); hint != "" {
				line += " " + mutedTextStyle.Render(hint)
			}
		}
		if i == p.cursor {
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}
