package tui

import (
	"fmt"
	"strings"

	"github.com/CaptShanks/terraprism/internal/history"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PickerModel is a TUI for selecting a history entry
type PickerModel struct {
	allEntries []history.Entry // Original unfiltered list
	filtered   []history.Entry // Filtered list based on search
	cursor     int
	selected   string // Path of selected entry
	quitting   bool
	height     int
	width      int

	// Search state
	searching   bool
	searchQuery string
}

// NewPickerModel creates a new history picker
func NewPickerModel(entries []history.Entry) PickerModel {
	return PickerModel{
		allEntries: entries,
		filtered:   entries,
		cursor:     0,
	}
}

// SelectedPath returns the path of the selected entry (empty if cancelled)
func (m PickerModel) SelectedPath() string {
	return m.selected
}

func (m PickerModel) Init() tea.Cmd {
	return nil
}

// filterEntries filters entries based on search query
// Supports fzf-style multi-term matching: "project apply success" matches all terms (AND)
func (m *PickerModel) filterEntries() {
	if m.searchQuery == "" {
		m.filtered = m.allEntries
		return
	}

	// Split query into terms (space-separated)
	terms := strings.Fields(strings.ToLower(m.searchQuery))
	if len(terms) == 0 {
		m.filtered = m.allEntries
		return
	}

	var results []history.Entry

	for _, entry := range m.allEntries {
		// Build searchable string from all fields
		searchable := strings.ToLower(
			entry.Project + " " +
				entry.Command + " " +
				entry.Status + " " +
				entry.Timestamp.Format("2006-01-02 15:04") + " " +
				entry.Filename + " " +
				entry.WorkingDir,
		)

		// All terms must match (AND logic, like fzf)
		allMatch := true
		for _, term := range terms {
			if !strings.Contains(searchable, term) {
				allMatch = false
				break
			}
		}

		if allMatch {
			results = append(results, entry)
		}
	}

	m.filtered = results
	// Reset cursor if out of bounds
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}
}

func (m *PickerModel) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searching = false
		m.searchQuery = ""
		m.filterEntries()
		return *m, nil, true
	case tea.KeyEnter:
		m.searching = false
		return *m, nil, true
	case tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.filterEntries()
		}
		return *m, nil, true
	case tea.KeyRunes:
		m.searchQuery += string(msg.Runes)
		m.filterEntries()
		return *m, nil, true
	case tea.KeySpace:
		m.searchQuery += " "
		m.filterEntries()
		return *m, nil, true
	}
	return *m, nil, false
}

func (m *PickerModel) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	if key.Matches(msg, key.NewBinding(key.WithKeys("/"))) {
		m.searching = true
		return *m, nil, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))) {
		m.quitting = true
		return *m, tea.Quit, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("esc"))) {
		if m.searchQuery != "" {
			m.searchQuery = ""
			m.filterEntries()
			return *m, nil, true
		}
		m.quitting = true
		return *m, tea.Quit, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))) {
		if len(m.filtered) > 0 {
			m.selected = m.filtered[m.cursor].Path
		}
		m.quitting = true
		return *m, tea.Quit, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))) {
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
		return *m, nil, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))) {
		if m.cursor > 0 {
			m.cursor--
		}
		return *m, nil, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("d"))) {
		half := m.visibleRows() / 2
		if half < 1 {
			half = 1
		}
		m.cursor += half
		if m.cursor >= len(m.filtered) {
			m.cursor = len(m.filtered) - 1
		}
		return *m, nil, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("u"))) {
		half := m.visibleRows() / 2
		if half < 1 {
			half = 1
		}
		m.cursor -= half
		if m.cursor < 0 {
			m.cursor = 0
		}
		return *m, nil, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("g"))) {
		m.cursor = 0
		return *m, nil, true
	}
	if key.Matches(msg, key.NewBinding(key.WithKeys("G"))) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
		return *m, nil, true
	}
	return *m, nil, false
}

func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		if m.searching {
			newM, cmd, handled := (&m).handleSearchKey(msg)
			if handled {
				return newM, cmd
			}
			return m, nil
		}
		newM, cmd, handled := (&m).handlePickerKey(msg)
		if handled {
			return newM, cmd
		}
	}
	return m, nil
}

func (m PickerModel) visibleRows() int {
	rows := m.height - 8
	if rows < 5 {
		rows = 5
	}
	return rows
}

func pickerEntryStatus(status string) (label string, style lipgloss.Style) {
	s := lipgloss.NewStyle()
	switch status {
	case "success":
		return "[SUCCESS]", s.Foreground(lipgloss.Color("#a6e3a1"))
	case "failed":
		return "[FAILED]", s.Foreground(lipgloss.Color("#f38ba8"))
	case "cancelled":
		return "[CANCELLED]", s.Foreground(lipgloss.Color("#fab387"))
	default:
		return "", s
	}
}

func pickerTruncatePath(path string, maxLen int) string {
	if path == "" {
		return "-"
	}
	if len(path) > maxLen {
		return "..." + path[len(path)-maxLen+3:]
	}
	return path
}

func (m PickerModel) pickerViewEntries(b *strings.Builder) {
	if len(m.filtered) == 0 {
		noResultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Italic(true)
		if m.searchQuery != "" {
			b.WriteString(noResultStyle.Render(fmt.Sprintf("  No results for '%s'", m.searchQuery)))
		} else {
			b.WriteString(noResultStyle.Render("  No history entries"))
		}
		b.WriteString("\n")
		return
	}
	visibleRows := m.visibleRows()
	if visibleRows > len(m.filtered) {
		visibleRows = len(m.filtered)
	}
	startIdx, endIdx := 0, visibleRows
	if m.cursor >= visibleRows {
		startIdx = m.cursor - visibleRows + 1
		endIdx = startIdx + visibleRows
	}
	if endIdx > len(m.filtered) {
		endIdx = len(m.filtered)
		startIdx = endIdx - visibleRows
		if startIdx < 0 {
			startIdx = 0
		}
	}
	if startIdx > 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render(fmt.Sprintf("  ↑ %d more entries above", startIdx)))
		b.WriteString("\n")
	}
	for i := startIdx; i < endIdx; i++ {
		m.pickerWriteEntryLine(b, i)
	}
	if endIdx < len(m.filtered) {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render(fmt.Sprintf("  ↓ %d more entries below", len(m.filtered)-endIdx)))
		b.WriteString("\n")
	}
}

func (m PickerModel) pickerWriteEntryLine(b *strings.Builder, i int) {
	entry := m.filtered[i]
	cursor, style := "  ", lipgloss.NewStyle()
	if i == m.cursor {
		cursor = "> "
		style = lipgloss.NewStyle().Background(lipgloss.Color("#313244")).Foreground(lipgloss.Color("#cdd6f4")).Bold(true)
	}
	statusLabel, statusStyle := pickerEntryStatus(entry.Status)
	path := pickerTruncatePath(entry.WorkingDir, 40)
	if i == m.cursor {
		line := fmt.Sprintf("%s%2d  %s  %-7s  %-12s  %s", cursor, i+1, entry.Timestamp.Format("2006-01-02 15:04"), entry.Command, statusLabel, path)
		if len(line) < 95 {
			line += strings.Repeat(" ", 95-len(line))
		}
		b.WriteString(style.Render(line))
	} else {
		baseLine := fmt.Sprintf("%s%2d  %s  %-7s  ", cursor, i+1, entry.Timestamp.Format("2006-01-02 15:04"), entry.Command)
		b.WriteString(baseLine)
		b.WriteString(statusStyle.Render(fmt.Sprintf("%-12s", statusLabel)))
		b.WriteString("  " + path)
	}
	b.WriteString("\n")
}

func (m PickerModel) pickerViewFooter(b *strings.Builder) {
	if m.searching {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Bold(true).Render("/ "))
		b.WriteString(m.searchQuery)
		b.WriteString("█")
		return
	}
	if m.searchQuery != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Render(fmt.Sprintf("Filter: %s", m.searchQuery)))
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render(fmt.Sprintf("  (%d/%d)", len(m.filtered), len(m.allEntries))))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("j/k/↑↓: navigate  d/u: scroll  enter: select  esc: clear filter  q: cancel"))
		return
	}
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Render("j/k/↑↓: navigate  d/u: scroll  /: search  enter: select  q: cancel"))
}

func (m PickerModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#89b4fa")).MarginBottom(1).Render("Select a history entry to view"))
	b.WriteString("\n\n")
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086")).Bold(true)
	b.WriteString(headerStyle.Render("     TIMESTAMP        COMMAND  STATUS        PATH"))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render(strings.Repeat("─", 95)))
	b.WriteString("\n")
	m.pickerViewEntries(&b)
	b.WriteString("\n")
	m.pickerViewFooter(&b)
	return b.String()
}

// RunPicker runs the interactive history picker and returns the selected path
func RunPicker(entries []history.Entry) (string, error) {
	m := NewPickerModel(entries)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	pickerModel := finalModel.(PickerModel)
	return pickerModel.SelectedPath(), nil
}
