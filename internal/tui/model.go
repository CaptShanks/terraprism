package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/CaptShanks/terraprism/internal/parser"
)

// Model represents the TUI state
type Model struct {
	plan           *parser.Plan
	cursor         int
	expanded       map[int]bool
	viewport       viewport.Model
	ready          bool
	width          int
	height         int
	searching      bool
	searchInput    textinput.Model
	searchQuery    string
	searchMatches  []int
	currentMatch   int
	pendingG       bool // Track if 'g' was pressed, waiting for second 'g'

	// Apply mode fields
	applyMode    bool   // Whether apply is available
	planFile     string // Path to the plan file
	tfCommand    string // "terraform" or "tofu"
	shouldApply  bool   // User pressed 'a' to apply
	confirmApply bool   // Waiting for confirmation
}

// NewModel creates a new TUI model (view-only mode)
func NewModel(plan *parser.Plan) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100
	ti.Width = 40

	return Model{
		plan:          plan,
		expanded:      make(map[int]bool),
		searchInput:   ti,
		searchMatches: []int{},
		applyMode:     false,
	}
}

// NewModelWithApply creates a TUI model with apply capability
func NewModelWithApply(plan *parser.Plan, planFile, tfCommand string) Model {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100
	ti.Width = 40

	return Model{
		plan:          plan,
		expanded:      make(map[int]bool),
		searchInput:   ti,
		searchMatches: []int{},
		applyMode:     true,
		planFile:      planFile,
		tfCommand:     tfCommand,
	}
}

// ShouldApply returns true if user chose to apply
func (m Model) ShouldApply() bool {
	return m.shouldApply
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 4 // Title + summary + blank line
		footerHeight := 3 // Help text
		
		if !m.ready {
			m.viewport = viewport.New(msg.Width-4, msg.Height-headerHeight-footerHeight)
			m.viewport.YPosition = headerHeight
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - headerHeight - footerHeight
		}
		m.updateViewportContent()

	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "enter":
				m.searching = false
				m.searchQuery = m.searchInput.Value()
				m.performSearch()
				m.updateViewportContent()
			case "esc":
				m.searching = false
				m.searchInput.SetValue("")
				m.searchQuery = ""
				m.searchMatches = []int{}
				m.updateViewportContent()
			default:
				m.searchInput, cmd = m.searchInput.Update(msg)
				cmds = append(cmds, cmd)
			}
		} else {
			return m.handleNormalKey(msg)
		}

	case tea.MouseMsg:
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleNormalKey handles key presses in normal (non-search) mode
func (m Model) handleNormalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Reset pending g if any other key is pressed (except g itself)
	if msg.String() != "g" && msg.String() != "G" {
		m.pendingG = false
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.updateViewportContent()
			m.ensureCursorVisible()
		}

	case "down", "j":
		if m.cursor < len(m.plan.Resources)-1 {
			m.cursor++
			m.updateViewportContent()
			m.ensureCursorVisible()
		}

	case "enter", " ":
		m.expanded[m.cursor] = !m.expanded[m.cursor]
		m.updateViewportContent()
		m.ensureCursorVisible()

	case "e":
		m.expandAll()

	case "c":
		m.collapseAll()

	case "/":
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case "n":
		m.nextMatch()

	case "N":
		m.prevMatch()

	case "esc":
		m.clearSearch()

	case "backspace", "h", "left":
		m.expanded[m.cursor] = false
		m.updateViewportContent()
		m.ensureCursorVisible()

	case "d", "ctrl+d":
		m.scrollHalfPageDown()

	case "u", "ctrl+u":
		m.scrollHalfPageUp()

	case "g":
		m.handleGKey()

	case "G":
		m.gotoBottom()

	case "pgup":
		m.viewport.GotoTop()
		m.viewport.SetYOffset(m.viewport.YOffset - m.viewport.Height)

	case "pgdown":
		m.viewport.SetYOffset(m.viewport.YOffset + m.viewport.Height)

	case "l", "right":
		m.expanded[m.cursor] = true
		m.updateViewportContent()
		m.ensureCursorVisible()

	case "a":
		// Apply (only in apply mode)
		if m.applyMode {
			if m.confirmApply {
				// Already confirming, execute apply
				m.shouldApply = true
				return m, tea.Quit
			}
			// Start confirmation
			m.confirmApply = true
			m.updateViewportContent()
		}

	case "y":
		// Confirm apply
		if m.applyMode && m.confirmApply {
			m.shouldApply = true
			return m, tea.Quit
		}
	}

	// Cancel confirmation on any other key if confirming
	if m.confirmApply && msg.String() != "a" && msg.String() != "y" {
		m.confirmApply = false
		m.updateViewportContent()
	}

	return m, nil
}

// expandAll expands all resources
func (m *Model) expandAll() {
	for i := range m.plan.Resources {
		m.expanded[i] = true
	}
	m.updateViewportContent()
	m.ensureCursorVisible()
}

// collapseAll collapses all resources
func (m *Model) collapseAll() {
	for i := range m.plan.Resources {
		m.expanded[i] = false
	}
	m.updateViewportContent()
	m.ensureCursorVisible()
}

// nextMatch moves to the next search match
func (m *Model) nextMatch() {
	if len(m.searchMatches) > 0 {
		m.currentMatch = (m.currentMatch + 1) % len(m.searchMatches)
		m.cursor = m.searchMatches[m.currentMatch]
		m.updateViewportContent()
		m.ensureCursorVisible()
	}
}

// prevMatch moves to the previous search match
func (m *Model) prevMatch() {
	if len(m.searchMatches) > 0 {
		m.currentMatch--
		if m.currentMatch < 0 {
			m.currentMatch = len(m.searchMatches) - 1
		}
		m.cursor = m.searchMatches[m.currentMatch]
		m.updateViewportContent()
		m.ensureCursorVisible()
	}
}

// clearSearch clears the current search
func (m *Model) clearSearch() {
	m.searchQuery = ""
	m.searchMatches = []int{}
	m.searchInput.SetValue("")
	m.updateViewportContent()
}

// scrollHalfPageDown scrolls viewport half page down
func (m *Model) scrollHalfPageDown() {
	halfPage := m.viewport.Height / 2
	m.viewport.SetYOffset(m.viewport.YOffset + halfPage)
}

// scrollHalfPageUp scrolls viewport half page up
func (m *Model) scrollHalfPageUp() {
	halfPage := m.viewport.Height / 2
	newOffset := m.viewport.YOffset - halfPage
	if newOffset < 0 {
		newOffset = 0
	}
	m.viewport.SetYOffset(newOffset)
}

// handleGKey handles the g key for gg navigation
func (m *Model) handleGKey() {
	if m.pendingG {
		m.cursor = 0
		m.updateViewportContent()
		m.viewport.GotoTop()
		m.pendingG = false
	} else {
		m.pendingG = true
	}
}

// gotoBottom moves cursor to the last resource
func (m *Model) gotoBottom() {
	m.cursor = len(m.plan.Resources) - 1
	m.updateViewportContent()
	m.viewport.GotoBottom()
	m.pendingG = false
}

func (m *Model) performSearch() {
	m.searchMatches = []int{}
	m.currentMatch = 0
	
	if m.searchQuery == "" {
		return
	}

	query := strings.ToLower(m.searchQuery)
	for i, r := range m.plan.Resources {
		if strings.Contains(strings.ToLower(r.Address), query) ||
			strings.Contains(strings.ToLower(r.Type), query) ||
			strings.Contains(strings.ToLower(r.Name), query) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}

	if len(m.searchMatches) > 0 {
		m.cursor = m.searchMatches[0]
	}
}

func (m *Model) updateViewportContent() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderResources())
}

// ensureCursorVisible scrolls the viewport to make the current cursor visible
func (m *Model) ensureCursorVisible() {
	if !m.ready {
		return
	}

	// Calculate the line number where the current resource starts
	lineNum := 0
	for i := 0; i < m.cursor; i++ {
		lineNum++ // Resource header line
		if m.expanded[i] {
			// Add the content lines for expanded resources
			lineNum += len(m.plan.Resources[i].RawLines) // includes header + content
			lineNum++ // blank line after expanded resource
		}
	}

	// Get current viewport position
	topLine := m.viewport.YOffset
	bottomLine := topLine + m.viewport.Height - 1

	// Scroll if cursor is outside visible area
	if lineNum < topLine {
		// Cursor is above visible area - scroll up
		m.viewport.SetYOffset(lineNum)
	} else if lineNum > bottomLine {
		// Cursor is below visible area - scroll down
		newOffset := lineNum - m.viewport.Height + 1
		if newOffset < 0 {
			newOffset = 0
		}
		m.viewport.SetYOffset(newOffset)
	}
}

func (m Model) renderResources() string {
	var b strings.Builder

	for i, r := range m.plan.Resources {
		isSelected := i == m.cursor
		isExpanded := m.expanded[i]
		isMatch := false
		
		for _, match := range m.searchMatches {
			if match == i {
				isMatch = true
				break
			}
		}

		// Render resource line
		if isSelected {
			// For selected line, render with full-width background highlight
			line := m.renderSelectedResourceLine(r, isExpanded, isMatch)
			b.WriteString(line)
		} else {
			line := m.renderResourceLine(r, isExpanded, isMatch)
			b.WriteString(line)
		}
		b.WriteString("\n")

		// Render full HCL block if expanded
		if isExpanded && len(r.RawLines) > 1 {
			for _, line := range r.RawLines[1:] {
				coloredLine := m.colorizeHCLLine(line, r.Action)
				b.WriteString(coloredLine)
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderSelectedResourceLine renders a resource line with full-width background highlight
func (m Model) renderSelectedResourceLine(r parser.Resource, expanded bool, _ bool) string {
	// Build the line content
	var content strings.Builder

	// Expand/collapse indicator
	if expanded {
		content.WriteString("▼")
	} else {
		content.WriteString("▶")
	}
	content.WriteString(" ")

	// Action symbol
	switch r.Action {
	case parser.ActionCreate:
		content.WriteString("+")
	case parser.ActionDestroy:
		content.WriteString("-")
	case parser.ActionUpdate:
		content.WriteString("~")
	case parser.ActionReplace, parser.ActionDeleteCreate, parser.ActionCreateDelete:
		content.WriteString("±")
	case parser.ActionRead:
		content.WriteString("≤")
	default:
		content.WriteString("~")
	}
	content.WriteString(" ")

	// Resource address
	content.WriteString(r.Address)

	// Action description
	actionDesc := getActionDescription(r.Action)
	content.WriteString(" ")
	content.WriteString(actionDesc)

	// Line count
	if len(r.RawLines) > 1 {
		content.WriteString(fmt.Sprintf(" (%d lines)", len(r.RawLines)-1))
	}

	// Pad to full width and apply selected style with foreground color
	line := content.String()
	targetWidth := m.width - 4
	if targetWidth > 0 && len(line) < targetWidth {
		line = line + strings.Repeat(" ", targetWidth-len(line))
	}

	// Apply style with both foreground and background
	actionStyle := lipgloss.NewStyle().
		Background(selectedBg).
		Foreground(GetActionColor(string(r.Action))).
		Bold(true)

	return actionStyle.Render(line)
}

func (m Model) renderResourceLine(r parser.Resource, expanded bool, isMatch bool) string {
	var b strings.Builder

	// Expand/collapse indicator
	if expanded {
		b.WriteString(expandedIndicator)
	} else {
		b.WriteString(collapsedIndicator)
	}
	b.WriteString(" ")

	// Action symbol
	b.WriteString(GetActionSymbol(string(r.Action)))
	b.WriteString(" ")

	// Resource address
	style := GetResourceStyle(string(r.Action))
	address := r.Address
	
	if isMatch && m.searchQuery != "" {
		// Highlight matching text
		address = highlightMatch(address, m.searchQuery)
	}
	
	b.WriteString(style.Render(address))

	// Action description
	actionDesc := getActionDescription(r.Action)
	b.WriteString(" ")
	b.WriteString(mutedColor.Render(actionDesc))

	// Line count for expanded content
	if len(r.RawLines) > 1 {
		b.WriteString(mutedColor.Render(fmt.Sprintf(" (%d lines)", len(r.RawLines)-1)))
	}

	return b.String()
}

// colorizeHCLLine applies syntax highlighting to a line of HCL in the TUI
func (m Model) colorizeHCLLine(line string, action parser.Action) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	// Check for change symbols at start of content
	var prefix string
	var content string

	if strings.HasPrefix(trimmed, "+ ") {
		prefix = createSymbol
		content = trimmed[2:]
	} else if strings.HasPrefix(trimmed, "- ") {
		prefix = destroySymbol
		content = trimmed[2:]
	} else if strings.HasPrefix(trimmed, "~ ") {
		prefix = updateSymbol
		content = trimmed[2:]
	} else {
		prefix = " "
		content = trimmed
	}

	// Apply syntax highlighting
	coloredContent := m.colorizeHCLContent(content, action)

	return indent + prefix + " " + coloredContent
}

// colorizeHCLContent applies HCL syntax highlighting to content
func (m Model) colorizeHCLContent(content string, action parser.Action) string {
	// Empty or structural lines
	if content == "" || content == "{" || content == "}" || content == "]" || content == "[" {
		return mutedColor.Render(content)
	}

	// Check for key = value pattern
	if idx := strings.Index(content, " = "); idx > 0 {
		key := content[:idx]
		value := content[idx+3:]
		return attrNameStyle.Render(key) + " = " + m.colorizeValue(value, action)
	}

	// Nested block headers (e.g., "root_block_device {")
	if strings.HasSuffix(content, " {") {
		blockName := strings.TrimSuffix(content, " {")
		return lipgloss.NewStyle().Foreground(headerColor).Render(blockName) + " {"
	}

	// Resource declarations
	if strings.HasPrefix(content, "resource ") || strings.HasPrefix(content, "data ") {
		return lipgloss.NewStyle().Foreground(replaceColor).Bold(true).Render(content)
	}

	// Default
	return attrNameStyle.Render(content)
}

// colorizeValue applies coloring to a value based on its type
func (m Model) colorizeValue(value string, action parser.Action) string {
	value = strings.TrimSpace(value)

	// (known after apply)
	if strings.Contains(value, "(known after apply)") {
		return attrComputedStyle.Render(value)
	}

	// (sensitive value)
	if strings.Contains(value, "(sensitive") {
		return lipgloss.NewStyle().Foreground(replaceColor).Italic(true).Render(value)
	}

	// Change arrow: old -> new
	if strings.Contains(value, " -> ") {
		parts := strings.SplitN(value, " -> ", 2)
		oldVal := strings.TrimSpace(parts[0])
		newVal := strings.TrimSpace(parts[1])
		return attrOldValueStyle.Render(oldVal) + " → " + attrNewValueStyle.Render(newVal)
	}

	// null
	if value == "null" {
		return lipgloss.NewStyle().Foreground(destroyColor).Render(value)
	}

	// boolean
	if value == "true" || value == "false" {
		return lipgloss.NewStyle().Foreground(readColor).Render(value)
	}

	// Structural
	if value == "{" || value == "[" || strings.HasSuffix(value, "{") || strings.HasSuffix(value, "[") {
		return mutedColor.Render(value)
	}

	// Default based on action
	switch action {
	case parser.ActionCreate:
		return attrNewValueStyle.Render(value)
	case parser.ActionDestroy:
		return attrOldValueStyle.Render(value)
	default:
		return lipgloss.NewStyle().Foreground(textColor).Render(value)
	}
}

func highlightMatch(text, query string) string {
	lower := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	
	idx := strings.Index(lower, lowerQuery)
	if idx == -1 {
		return text
	}

	before := text[:idx]
	match := text[idx : idx+len(query)]
	after := text[idx+len(query):]

	return before + matchStyle.Render(match) + after
}

func getActionDescription(action parser.Action) string {
	switch action {
	case parser.ActionCreate:
		return "will be created"
	case parser.ActionDestroy:
		return "will be destroyed"
	case parser.ActionUpdate:
		return "will be updated"
	case parser.ActionReplace:
		return "must be replaced"
	case parser.ActionRead:
		return "will be read"
	case parser.ActionDeleteCreate:
		return "will be destroyed and then created"
	case parser.ActionCreateDelete:
		return "will be created and then destroyed"
	default:
		return ""
	}
}

// View renders the UI
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder

	// Header
	header := headerStyle.Render("🔺 Terra-Prism - Terraform Plan Viewer")
	b.WriteString(header)
	b.WriteString("\n")

	// Summary
	if m.plan.Summary != "" {
		summary := fmt.Sprintf("  %s to add, %s to change, %s to destroy",
			lipgloss.NewStyle().Foreground(createColor).Render(fmt.Sprintf("%d", m.plan.TotalAdd)),
			lipgloss.NewStyle().Foreground(updateColor).Render(fmt.Sprintf("%d", m.plan.TotalChange)),
			lipgloss.NewStyle().Foreground(destroyColor).Render(fmt.Sprintf("%d", m.plan.TotalDestroy)),
		)
		b.WriteString(summaryStyle.Render(summary))
	} else {
		b.WriteString(summaryStyle.Render(fmt.Sprintf("  %d resources with changes", len(m.plan.Resources))))
	}
	b.WriteString("\n\n")

	// Search bar (if active)
	if m.searching {
		b.WriteString(searchStyle.Render("Search: "))
		b.WriteString(m.searchInput.View())
		b.WriteString("\n\n")
	} else if m.searchQuery != "" {
		matchInfo := fmt.Sprintf("Search: %q (%d/%d matches)", m.searchQuery, m.currentMatch+1, len(m.searchMatches))
		b.WriteString(searchStyle.Render(matchInfo))
		b.WriteString("\n\n")
	}

	// Confirmation prompt (if confirming apply)
	if m.confirmApply {
		confirmStyle := lipgloss.NewStyle().
			Background(lipgloss.Color("#f38ba8")).
			Foreground(lipgloss.Color("#1e1e2e")).
			Bold(true).
			Padding(0, 2)
		b.WriteString("\n")
		b.WriteString(confirmStyle.Render("⚠️  Apply this plan? Press 'y' to confirm, any other key to cancel"))
		b.WriteString("\n\n")
	}

	// Viewport with resources
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Help footer
	var help string
	if m.applyMode {
		if m.confirmApply {
			help = "y: confirm apply • any key: cancel"
		} else {
			applyHint := lipgloss.NewStyle().Foreground(createColor).Bold(true).Render("a: APPLY")
			help = fmt.Sprintf("%s • j/k: navigate • e/c: all • /: search • q: quit", applyHint)
		}
	} else {
		help = "j/k: navigate • l/→: expand • h/←/⌫: collapse • d/u: scroll • e/c: all • gg/G: top/bottom • /: search • q: quit"
	}
	b.WriteString(helpStyle.Render(help))

	return appStyle.Render(b.String())
}

