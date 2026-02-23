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

func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
		return m, nil

	case tea.KeyMsg:
		// Handle search mode
		if m.searching {
			switch msg.Type {
			case tea.KeyEsc:
				m.searching = false
				m.searchQuery = ""
				m.filterEntries()
				return m, nil
			case tea.KeyEnter:
				m.searching = false
				return m, nil
			case tea.KeyBackspace:
				if len(m.searchQuery) > 0 {
					m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
					m.filterEntries()
				}
				return m, nil
			case tea.KeyRunes:
				m.searchQuery += string(msg.Runes)
				m.filterEntries()
				return m, nil
			case tea.KeySpace:
				m.searchQuery += " "
				m.filterEntries()
				return m, nil
			}
			return m, nil
		}

		// Normal mode
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("/"))):
			m.searching = true
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			if m.searchQuery != "" {
				// Clear search
				m.searchQuery = ""
				m.filterEntries()
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", " "))):
			if len(m.filtered) > 0 {
				m.selected = m.filtered[m.cursor].Path
			}
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			half := m.visibleRows() / 2
			if half < 1 {
				half = 1
			}
			m.cursor += half
			if m.cursor >= len(m.filtered) {
				m.cursor = len(m.filtered) - 1
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("u"))):
			half := m.visibleRows() / 2
			if half < 1 {
				half = 1
			}
			m.cursor -= half
			if m.cursor < 0 {
				m.cursor = 0
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
			m.cursor = 0
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
			if len(m.filtered) > 0 {
				m.cursor = len(m.filtered) - 1
			}
			return m, nil
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

func (m PickerModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Header
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#89b4fa")).
		MarginBottom(1)

	b.WriteString(titleStyle.Render("Select a history entry to view"))
	b.WriteString("\n\n")

	// Column headers
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6c7086")).
		Bold(true)

	b.WriteString(headerStyle.Render("     TIMESTAMP        COMMAND  STATUS        PATH"))
	b.WriteString("\n")
	b.WriteString(headerStyle.Render(strings.Repeat("─", 95)))
	b.WriteString("\n")

	// Entries with scroll window
	if len(m.filtered) == 0 {
		noResultStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086")).
			Italic(true)
		if m.searchQuery != "" {
			b.WriteString(noResultStyle.Render(fmt.Sprintf("  No results for '%s'", m.searchQuery)))
		} else {
			b.WriteString(noResultStyle.Render("  No history entries"))
		}
		b.WriteString("\n")
	} else {
		visibleRows := m.visibleRows()
		if visibleRows > len(m.filtered) {
			visibleRows = len(m.filtered)
		}

		// Determine scroll window
		startIdx := 0
		if m.cursor >= visibleRows {
			startIdx = m.cursor - visibleRows + 1
		}
		endIdx := startIdx + visibleRows
		if endIdx > len(m.filtered) {
			endIdx = len(m.filtered)
			startIdx = endIdx - visibleRows
			if startIdx < 0 {
				startIdx = 0
			}
		}

		if startIdx > 0 {
			scrollHint := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))
			b.WriteString(scrollHint.Render(fmt.Sprintf("  ↑ %d more entries above", startIdx)))
			b.WriteString("\n")
		}

		for i := startIdx; i < endIdx; i++ {
			entry := m.filtered[i]
			cursor := "  "
			style := lipgloss.NewStyle()

			if i == m.cursor {
				cursor = "> "
				style = lipgloss.NewStyle().
					Background(lipgloss.Color("#313244")).
					Foreground(lipgloss.Color("#cdd6f4")).
					Bold(true)
			}

			status := ""
			statusStyle := lipgloss.NewStyle()
			switch entry.Status {
			case "success":
				status = "[SUCCESS]"
				statusStyle = statusStyle.Foreground(lipgloss.Color("#a6e3a1"))
			case "failed":
				status = "[FAILED]"
				statusStyle = statusStyle.Foreground(lipgloss.Color("#f38ba8"))
			case "cancelled":
				status = "[CANCELLED]"
				statusStyle = statusStyle.Foreground(lipgloss.Color("#fab387"))
			}

			path := entry.WorkingDir
			if path == "" {
				path = "-"
			}
			if len(path) > 40 {
				path = "..." + path[len(path)-37:]
			}

			line := fmt.Sprintf("%s%2d  %s  %-7s  %-12s  %s",
				cursor,
				i+1,
				entry.Timestamp.Format("2006-01-02 15:04"),
				entry.Command,
				status,
				path,
			)

			if i == m.cursor {
				if len(line) < 95 {
					line = line + strings.Repeat(" ", 95-len(line))
				}
				b.WriteString(style.Render(line))
			} else {
				baseLine := fmt.Sprintf("%s%2d  %s  %-7s  ",
					cursor,
					i+1,
					entry.Timestamp.Format("2006-01-02 15:04"),
					entry.Command,
				)
				b.WriteString(baseLine)
				b.WriteString(statusStyle.Render(fmt.Sprintf("%-12s", status)))
				b.WriteString("  " + path)
			}
			b.WriteString("\n")
		}

		if endIdx < len(m.filtered) {
			scrollHint := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))
			b.WriteString(scrollHint.Render(fmt.Sprintf("  ↓ %d more entries below", len(m.filtered)-endIdx)))
			b.WriteString("\n")
		}
	}

	// Search bar / Footer
	b.WriteString("\n")

	if m.searching {
		searchStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af")).
			Bold(true)
		b.WriteString(searchStyle.Render("/ "))
		b.WriteString(m.searchQuery)
		b.WriteString("█") // Cursor
	} else if m.searchQuery != "" {
		// Show active filter
		filterStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f9e2af"))
		countStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086"))
		b.WriteString(filterStyle.Render(fmt.Sprintf("Filter: %s", m.searchQuery)))
		b.WriteString(countStyle.Render(fmt.Sprintf("  (%d/%d)", len(m.filtered), len(m.allEntries))))
		b.WriteString("\n")
		footerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086"))
		b.WriteString(footerStyle.Render("j/k/↑↓: navigate  d/u: scroll  enter: select  esc: clear filter  q: cancel"))
	} else {
		footerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6c7086"))
		b.WriteString(footerStyle.Render("j/k/↑↓: navigate  d/u: scroll  /: search  enter: select  q: cancel"))
	}

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
