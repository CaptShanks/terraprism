// Command foldtree-demo is an interactive playground for
// internal/foldtree, entirely independent of terraprism's plan data.
// It wires a foldtree.State directly to a Bubble Tea program over a
// handful of sample tree shapes (flat, nested, deep, wide, multiline,
// and deliberately degenerate) so the navigation feel can be tried by
// hand without touching any Terraform-specific code.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/CaptShanks/terraprism/internal/foldtree"
)

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("236")).Foreground(lipgloss.Color("212")).Width(1000)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
)

type model struct {
	vIdx  int
	state *foldtree.State
	reg   map[string]content

	vp    viewport.Model
	ready bool
}

func newModel() model {
	m := model{state: foldtree.New(10)}
	m.loadVariant(0)
	return m
}

func (m *model) loadVariant(i int) {
	m.vIdx = i
	roots, reg := variants[i].build()
	m.reg = reg
	m.state.SetTree(roots)
}

func (m *model) toggleSelected() {
	idx := m.state.SelectedIndex()
	if idx < 0 {
		return
	}
	row := m.state.Rows()[idx]
	if row.HasChildren {
		m.state.ToggleCollapse(row.ID)
	}
}

// expandCurrent recursively expands everything under the current node,
// scoped to exactly this node's branch.
func (m *model) expandCurrent() {
	if id, ok := m.state.SelectedID(); ok {
		m.state.ExpandSubtree(id)
	}
}

// collapseCurrent recursively collapses the current node's whole
// branch. If there's nothing left to collapse there (a leaf, or already
// collapsed), it climbs to the parent and collapses that branch
// instead, moving the selection up with it — so repeated presses walk
// up the tree one level at a time. Once there's no parent left to climb
// to (already at a root), it falls back to collapsing the whole tree,
// same as pressing C directly.
func (m *model) collapseCurrent() {
	idx := m.state.SelectedIndex()
	if idx < 0 {
		return
	}
	row := m.state.Rows()[idx]
	if row.HasChildren && !row.Collapsed {
		m.state.CollapseSubtree(row.ID)
		return
	}

	id := row.ID
	m.state.SelectParent()
	parentID, ok := m.state.SelectedID()
	if !ok || parentID == id {
		m.state.CollapseAll()
		return
	}
	m.state.CollapseSubtree(parentID)
}

func (m *model) refreshContent() {
	m.vp.SetContent(renderRows(m.state, m.reg, m.vp.Width))
	m.vp.SetYOffset(m.state.Offset())
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		const chromeLines = 2 // header + help line
		height := msg.Height - chromeLines
		if height < 1 {
			height = 1
		}
		if !m.ready {
			m.vp = viewport.New(msg.Width, height)
			m.ready = true
		} else {
			m.vp.Width = msg.Width
			m.vp.Height = height
		}
		m.state.SetHeight(height)
		m.refreshContent()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.state.MoveUp()
		case "down", "j":
			m.state.MoveDown()
		case "pgup":
			m.state.PageUp()
		case "pgdown":
			m.state.PageDown()
		case "g":
			m.state.MoveToTop()
		case "G":
			m.state.MoveToBottom()
		case "p":
			m.state.SelectParent()
		case "enter", " ":
			m.toggleSelected() // toggle exactly the selected row, one level
		case "e":
			m.expandCurrent() // this node's whole branch, recursively
		case "c":
			m.collapseCurrent() // this node's whole branch; climbs a level once already collapsed
		case "E":
			m.state.ExpandAll() // everything, globally
		case "C":
			m.state.CollapseAll() // everything, globally
		case "tab":
			m.loadVariant((m.vIdx + 1) % len(variants))
		default:
			if n, err := strconv.Atoi(msg.String()); err == nil && n >= 1 && n <= len(variants) {
				m.loadVariant(n - 1)
			}
		}
		m.refreshContent()

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.state.MoveMouse(-3)
			m.refreshContent()
		case tea.MouseButtonWheelDown:
			m.state.MoveMouse(3)
			m.refreshContent()
		}
	}
	return m, nil
}

func (m model) View() string {
	if !m.ready {
		return "initializing..."
	}
	id, _ := m.state.SelectedID()
	header := headerStyle.Render(fmt.Sprintf(" [%d/%d] %s  |  rows=%d cursor=%d id=%q offset=%d total=%d ",
		m.vIdx+1, len(variants), variants[m.vIdx].name,
		len(m.state.Rows()), m.state.SelectedIndex(), id, m.state.Offset(), m.state.TotalLines()))
	help := helpStyle.Render("↑/k ↓/j move  pgup/pgdn page  g/G top/bottom  p parent  enter/space toggle (1 level)  " +
		"e/c expand/collapse this node's whole branch (c climbs up a level once collapsed)  " +
		"E/C expand/collapse everything  tab/1-7 switch tree  q quit")
	return header + "\n" + m.vp.View() + "\n" + help
}

// renderRows renders every currently visible row, padding each line to
// width and highlighting the selected row's lines, then returns it as
// one string for the viewport to display and scroll via YOffset.
func renderRows(s *foldtree.State, reg map[string]content, width int) string {
	rows := s.Rows()
	selected := s.SelectedIndex()

	var out []string
	for i, row := range rows {
		lines := reg[row.ID].lines
		indent := strings.Repeat("  ", row.Depth)
		marker := "  "
		if row.HasChildren {
			if row.Collapsed {
				marker = "▶ "
			} else {
				marker = "▼ "
			}
		}

		for li := 0; li < row.Height; li++ {
			var text string
			if li < len(lines) {
				text = lines[li]
			}
			prefix := indent + "  "
			if li == 0 {
				prefix = indent + marker
			}
			line := prefix + text
			if width > 0 {
				line = padTo(line, width)
			}
			if i == selected {
				line = selectedStyle.Render(line)
			}
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func padTo(s string, width int) string {
	if n := lipgloss.Width(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

func main() {
	// --rich is a separate startup mode, not an in-program toggle: Bubble
	// Tea can't hot-swap a running program's root model, so choosing
	// between the bare-State demo and the fuller TreeView/Picker demo has
	// to happen before tea.NewProgram is ever constructed.
	if len(os.Args) > 1 && os.Args[1] == "--rich" {
		runRichDemo()
		return
	}

	p := tea.NewProgram(newModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
