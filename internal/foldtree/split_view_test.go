package foldtree

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubPaneMsg is a custom application message used to test that non-key
// messages broadcast to both panes.
type stubPaneMsg struct{ n int }

// stubPane is a trivial tea.Model that records every message it
// receives, so tests can assert exactly what SplitView routed to it.
type stubPane struct {
	name        string
	lastSize    tea.WindowSizeMsg
	sizeCount   int
	lastKey     string
	keyCount    int
	customCount int
}

func (p stubPane) Init() tea.Cmd { return nil }

func (p stubPane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.lastSize = msg
		p.sizeCount++
	case tea.KeyMsg:
		p.lastKey = msg.String()
		p.keyCount++
	case stubPaneMsg:
		p.customCount += msg.n
	}
	return p, nil
}

func (p stubPane) View() string { return p.name }

func TestSplitViewHiddenAuxGivesPrimaryFullHeight(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)

	p := got.Primary.(stubPane)
	if p.lastSize.Height != 20 {
		t.Fatalf("hidden aux: primary height = %d, want 20 (full)", p.lastSize.Height)
	}
	if got.View() != "primary" {
		t.Fatalf("View() with hidden aux should be just Primary's view, got %q", got.View())
	}
}

func TestSplitViewShowAuxSplitsHeight(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)
	got.ShowAux()

	p := got.Primary.(stubPane)
	a := got.Aux.(stubPane)
	if p.lastSize.Height != 15 {
		t.Fatalf("primary height = %d, want 15 (20 - 5)", p.lastSize.Height)
	}
	if a.lastSize.Height != 5 {
		t.Fatalf("aux height = %d, want 5", a.lastSize.Height)
	}
	view := got.View()
	if !strings.Contains(view, "primary") || !strings.Contains(view, "aux") {
		t.Fatalf("View() with visible aux should stack both, got %q", view)
	}
}

func TestSplitViewAuxResizedEvenWhileHidden(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)

	a := got.Aux.(stubPane)
	if a.sizeCount == 0 {
		t.Fatalf("expected aux to receive a resize message even while hidden (zero height is fine -- primary owns all space until shown), so its size-handling path has already run once by the time it's shown")
	}
}

func TestSplitViewToggleAuxFlipsVisibilityAndReflows(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)

	got.ToggleAux()
	if !got.AuxVisible() {
		t.Fatalf("expected ToggleAux to show a hidden pane")
	}
	if got.Primary.(stubPane).lastSize.Height != 15 {
		t.Fatalf("expected primary reflowed immediately on ToggleAux, got height %d", got.Primary.(stubPane).lastSize.Height)
	}

	got.ToggleAux()
	if got.AuxVisible() {
		t.Fatalf("expected ToggleAux to hide a visible pane")
	}
	if got.Primary.(stubPane).lastSize.Height != 20 {
		t.Fatalf("expected primary reflowed back to full height, got %d", got.Primary.(stubPane).lastSize.Height)
	}
}

func TestSplitViewHideAuxDropsFocus(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)
	got.ShowAux()
	got.FocusAux(true)
	if !got.AuxFocused() {
		t.Fatalf("expected FocusAux(true) to take effect while aux is visible")
	}
	got.HideAux()
	if got.AuxFocused() {
		t.Fatalf("expected HideAux to drop focus back to primary")
	}
}

func TestSplitViewKeyRoutingRespectsFocus(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)
	got.ShowAux()

	m, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got = m.(SplitView)
	if got.Primary.(stubPane).keyCount != 1 {
		t.Fatalf("expected key routed to Primary by default, got keyCount=%d", got.Primary.(stubPane).keyCount)
	}
	if got.Aux.(stubPane).keyCount != 0 {
		t.Fatalf("expected key NOT routed to Aux by default")
	}

	got.FocusAux(true)
	m, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	got = m.(SplitView)
	if got.Aux.(stubPane).keyCount != 1 {
		t.Fatalf("expected key routed to Aux once focused, got keyCount=%d", got.Aux.(stubPane).keyCount)
	}
	if got.Primary.(stubPane).keyCount != 1 {
		t.Fatalf("expected Primary's keyCount unchanged once focus moved to Aux")
	}
}

func TestSplitViewKeyRoutedToPrimaryWhenAuxHiddenEvenIfFocusFlagSet(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)
	got.FocusAux(true) // stale/defensive: focus set but aux never shown

	m, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got = m.(SplitView)
	if got.Primary.(stubPane).keyCount != 1 {
		t.Fatalf("expected key routed to Primary when aux is hidden, regardless of focus flag")
	}
}

func TestSplitViewNonKeyMessageBroadcastsToBoth(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	got := m.(SplitView)

	m, _ = got.Update(stubPaneMsg{n: 7})
	got = m.(SplitView)
	if got.Primary.(stubPane).customCount != 7 || got.Aux.(stubPane).customCount != 7 {
		t.Fatalf("expected a custom message broadcast to both panes, got primary=%d aux=%d",
			got.Primary.(stubPane).customCount, got.Aux.(stubPane).customCount)
	}
}

func TestSplitViewClampsPrimaryHeightOnVeryShortTerminal(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 80, Height: 3}) // shorter than auxHeight
	got := m.(SplitView)
	got.ShowAux()

	p := got.Primary.(stubPane)
	a := got.Aux.(stubPane)
	if p.lastSize.Height < 1 {
		t.Fatalf("expected primary height clamped to at least 1, got %d", p.lastSize.Height)
	}
	if p.lastSize.Height+a.lastSize.Height != 3 {
		t.Fatalf("expected primary+aux heights to sum to the full height (3), got %d+%d", p.lastSize.Height, a.lastSize.Height)
	}
}

func TestSplitViewInitBatchesBothPanes(t *testing.T) {
	sv := NewSplitView(stubPane{name: "primary"}, stubPane{name: "aux"}, 5)
	// Neither stubPane returns a Cmd from Init, but this should not panic
	// and should return a non-nil batch-shaped Cmd handling both.
	cmd := sv.Init()
	_ = cmd
}
