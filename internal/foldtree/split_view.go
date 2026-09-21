package foldtree

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SplitView composes a primary pane with a collapsible auxiliary pane
// stacked vertically. The aux pane occupies zero height until shown;
// showing/hiding reflows the primary pane's height so the two always
// exactly fill whatever size SplitView itself was given, never overlap.
//
// SplitView is fully generic over tea.Model — it has no dependency on
// TreeView or LogPane, so it's independently reusable for any two panes
// a host wants to stack this way.
type SplitView struct {
	Primary tea.Model
	Aux     tea.Model

	auxVisible bool
	auxHeight  int // fixed line count when visible
	auxFocused bool

	width      int
	fullHeight int
	sized      bool
}

var _ tea.Model = SplitView{}

// NewSplitView returns a SplitView with Aux initially hidden. auxHeight
// is how many lines Aux occupies once shown (clamped down on very short
// terminals — see resize).
func NewSplitView(primary, aux tea.Model, auxHeight int) *SplitView {
	if auxHeight < 0 {
		auxHeight = 0
	}
	return &SplitView{Primary: primary, Aux: aux, auxHeight: auxHeight}
}

// AuxVisible reports whether the auxiliary pane is currently shown.
func (s *SplitView) AuxVisible() bool { return s.auxVisible }

// ShowAux shows the auxiliary pane and reflows both panes' heights.
func (s *SplitView) ShowAux() {
	s.auxVisible = true
	s.reflow()
}

// HideAux hides the auxiliary pane, gives its space back to Primary, and
// drops its focus (keys always go back to Primary once Aux is hidden).
func (s *SplitView) HideAux() {
	s.auxVisible = false
	s.auxFocused = false
	s.reflow()
}

// ToggleAux flips auxiliary-pane visibility.
func (s *SplitView) ToggleAux() {
	if s.auxVisible {
		s.HideAux()
	} else {
		s.ShowAux()
	}
}

// FocusAux moves key routing to (or back from) the auxiliary pane. Only
// meaningful while AuxVisible(); a host would typically bind a single key
// (e.g. Tab) to FocusAux(!focused).
func (s *SplitView) FocusAux(focused bool) { s.auxFocused = focused }

// AuxFocused reports whether keys are currently routed to Aux.
func (s *SplitView) AuxFocused() bool { return s.auxFocused }

// reflow (re)computes each pane's height from the last known size and
// pushes a resize into both — needed not just on an actual terminal
// resize but also whenever ShowAux/HideAux/ToggleAux flips visibility,
// since that changes the split without a new tea.WindowSizeMsg arriving.
// Aux is resized even while hidden, so it's immediately ready the instant
// it's shown.
func (s *SplitView) reflow() {
	if !s.sized {
		return
	}
	primaryHeight := s.fullHeight
	auxHeight := 0
	if s.auxVisible {
		auxHeight = s.auxHeight
		primaryHeight = s.fullHeight - auxHeight
	}
	if primaryHeight < 1 {
		primaryHeight = 1
	}
	auxHeight = s.fullHeight - primaryHeight
	if auxHeight < 0 {
		auxHeight = 0
	}

	var cmd tea.Cmd
	s.Primary, cmd = s.Primary.Update(tea.WindowSizeMsg{Width: s.width, Height: primaryHeight})
	_ = cmd // no async work expected from a resize; nothing to run here
	s.Aux, cmd = s.Aux.Update(tea.WindowSizeMsg{Width: s.width, Height: auxHeight})
	_ = cmd
}

// Init implements tea.Model.
func (s SplitView) Init() tea.Cmd {
	return tea.Batch(s.Primary.Init(), s.Aux.Init())
}

// Update implements tea.Model. tea.WindowSizeMsg and any non-key message
// (including custom application messages, e.g. streamed process output)
// go to both panes unconditionally; tea.KeyMsg goes to whichever pane
// currently has focus (Primary by default).
func (s SplitView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.width, s.fullHeight = msg.Width, msg.Height
		s.sized = true
		s.reflow()
		return s, nil

	case tea.KeyMsg:
		var cmd tea.Cmd
		if s.auxFocused && s.auxVisible {
			s.Aux, cmd = s.Aux.Update(msg)
		} else {
			s.Primary, cmd = s.Primary.Update(msg)
		}
		return s, cmd

	default:
		var cmds []tea.Cmd
		var cmd tea.Cmd
		s.Primary, cmd = s.Primary.Update(msg)
		cmds = append(cmds, cmd)
		s.Aux, cmd = s.Aux.Update(msg)
		cmds = append(cmds, cmd)
		return s, tea.Batch(cmds...)
	}
}

// View implements tea.Model.
func (s SplitView) View() string {
	if !s.auxVisible {
		return s.Primary.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, s.Primary.View(), s.Aux.View())
}
