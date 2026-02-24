package tui

import (
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"github.com/CaptShanks/terraprism/internal/parser"
)

// Model represents the TUI state
type Model struct {
	plan          *parser.Plan
	cursor        int
	expanded      map[int]bool
	viewport      viewport.Model
	ready         bool
	width         int
	height        int
	searching     bool
	searchInput   textinput.Model
	searchQuery   string
	searchMatches []int
	currentMatch  int
	pendingG           bool  // Track if 'g' was pressed, waiting for second 'g'
	resourceLineStarts []int // rendered line offset per resource (populated during render)
	contentLineCount   int   // total rendered content lines (excluding padding)

	// Apply mode fields
	applyMode    bool   // Whether apply is available
	planFile     string // Path to the plan file
	tfCommand    string // "terraform" or "tofu"
	shouldApply  bool   // User pressed 'a' to apply
	confirmApply bool   // Waiting for confirmation

	// Status filter fields
	statusFilters map[parser.Action]bool // true = show resources with this action
	filtering     bool                    // filter picker is open
	filterCursor  int                     // cursor in filter picker

	// Sort fields
	sortOrder   SortOrder // default, byAction, byAddress, byType
	sorting     bool      // sort picker is open
	sortCursor  int       // cursor in sort picker
}

// SortOrder defines how resources are ordered
type SortOrder string

const (
	SortDefault   SortOrder = "default"
	SortByAction  SortOrder = "action"
	SortByAddress SortOrder = "address"
	SortByType    SortOrder = "type"
)

// sortOptions is the ordered list of sort choices for the picker
var sortOptions = []SortOrder{SortDefault, SortByAction, SortByAddress, SortByType}

// actionOrder defines sort order for actions (destructive last)
var actionOrder = map[parser.Action]int{
	parser.ActionCreate:       0,
	parser.ActionRead:         1,
	parser.ActionUpdate:       2,
	parser.ActionReplace:      3,
	parser.ActionDeleteCreate: 4,
	parser.ActionCreateDelete: 5,
	parser.ActionDestroy:      6,
	parser.ActionNoOp:         7,
}

// filterableActions is the ordered list of statuses available for filtering
var filterableActions = []parser.Action{
	parser.ActionCreate,
	parser.ActionDestroy,
	parser.ActionUpdate,
	parser.ActionReplace,
	parser.ActionRead,
	parser.ActionDeleteCreate,
	parser.ActionCreateDelete,
}

// filteredResources returns indices into plan.Resources that pass the status filter.
// When statusFilters is empty or nil, returns all indices.
func (m *Model) filteredResources() []int {
	if len(m.statusFilters) == 0 {
		indices := make([]int, len(m.plan.Resources))
		for i := range m.plan.Resources {
			indices[i] = i
		}
		return indices
	}
	var indices []int
	for i, r := range m.plan.Resources {
		if m.statusFilters[r.Action] {
			indices = append(indices, i)
		}
	}
	return indices
}

// sortedResources returns filtered indices sorted by the current sort order.
func (m *Model) sortedResources() []int {
	filtered := m.filteredResources()
	if m.sortOrder == SortDefault || m.sortOrder == "" {
		return filtered
	}
	sort.Slice(filtered, func(i, j int) bool {
		ri := m.plan.Resources[filtered[i]]
		rj := m.plan.Resources[filtered[j]]
		switch m.sortOrder {
		case SortByAction:
			oi, oki := actionOrder[ri.Action]
			oj, okj := actionOrder[rj.Action]
			if !oki {
				oi = 99
			}
			if !okj {
				oj = 99
			}
			if oi != oj {
				return oi < oj
			}
			return ri.Address < rj.Address
		case SortByAddress:
			return ri.Address < rj.Address
		case SortByType:
			if ri.Type != rj.Type {
				return ri.Type < rj.Type
			}
			return ri.Address < rj.Address
		}
		return false
	})
	return filtered
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
		statusFilters: nil, // nil = show all
		sortOrder:     SortDefault,
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
		statusFilters: nil, // nil = show all
		sortOrder:     SortDefault,
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
		if m.filtering {
			return m.handleFilterKey(msg)
		}
		if m.sorting {
			return m.handleSortKey(msg)
		}
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
		} else {
			m.viewport.SetYOffset(m.viewport.YOffset - 1)
		}

	case "down", "j":
		filtered := m.sortedResources()
		if m.cursor < len(filtered)-1 {
			m.cursor++
			m.updateViewportContent()
			m.ensureCursorVisible()
		} else {
			m.viewport.SetYOffset(m.viewport.YOffset + 1)
		}

	case "enter", " ":
		filtered := m.sortedResources()
		if len(filtered) > 0 && m.cursor >= 0 && m.cursor < len(filtered) {
			resourceIdx := filtered[m.cursor]
			m.expanded[resourceIdx] = !m.expanded[resourceIdx]
		}
		m.updateViewportContent()
		m.scrollForExpanded()

	case "e":
		m.expandAll()

	case "c":
		m.collapseAll()

	case "f":
		m.filtering = true
		m.filterCursor = 0
		// Ensure statusFilters map exists when opening picker
		if m.statusFilters == nil {
			m.statusFilters = make(map[parser.Action]bool)
		}
		return m, nil

	case "s":
		m.sorting = true
		// Set sortCursor to current sort order
		m.sortCursor = 0
		for i, opt := range sortOptions {
			if opt == m.sortOrder {
				m.sortCursor = i
				break
			}
		}
		return m, nil

	case "/":
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink

	case "n":
		m.nextMatch()

	case "N":
		m.prevMatch()

	case "esc":
		if len(m.statusFilters) > 0 {
			m.statusFilters = nil
			m.clampCursorAndRefreshSearch()
			m.updateViewportContent()
		} else {
			m.clearSearch()
		}

	case "backspace", "h", "left":
		filtered := m.sortedResources()
		if len(filtered) > 0 && m.cursor >= 0 && m.cursor < len(filtered) {
			m.expanded[filtered[m.cursor]] = false
		}
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
		filtered := m.sortedResources()
		if len(filtered) > 0 && m.cursor >= 0 && m.cursor < len(filtered) {
			m.expanded[filtered[m.cursor]] = true
		}
		m.updateViewportContent()
		m.scrollForExpanded()

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

// handleFilterKey handles key presses in filter picker mode
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.statusFilters = nil
		m.filtering = false
		m.clampCursorAndRefreshSearch()
		m.updateViewportContent()
		return m, nil

	case "enter":
		// Toggle on Space, apply and close on Enter (when not toggling)
		// Enter toggles too per plan - "Space/Enter: toggle selected status on/off"
		// So Enter both toggles and... the plan says "Enter: Apply and close". Let me re-read.
		// "Space/Enter: toggle selected status on/off" and "Enter: Apply and close"
		// So Enter toggles the current selection AND applies/closes? Or Enter just applies?
		// Typical UX: Space toggles, Enter applies and closes. So we need to not toggle on Enter, just close.
		// Actually "Enter (when not toggling): apply filters and close" - so Enter = apply and close, don't toggle.
		m.filtering = false
		m.clampCursorAndRefreshSearch()
		m.updateViewportContent()
		return m, nil

	case "up", "k":
		if m.filterCursor > 0 {
			m.filterCursor--
		}
		return m, nil

	case "down", "j":
		if m.filterCursor < len(filterableActions)-1 {
			m.filterCursor++
		}
		return m, nil

	case " ":
		// Space toggles the selected status
		action := filterableActions[m.filterCursor]
		m.statusFilters[action] = !m.statusFilters[action]
		return m, nil

	case "a":
		// Select all
		for _, action := range filterableActions {
			m.statusFilters[action] = true
		}
		return m, nil

	case "c":
		// Clear all filters (show all)
		m.statusFilters = make(map[parser.Action]bool)
		return m, nil
	}

	return m, nil
}

// handleSortKey handles key presses in sort picker mode
func (m Model) handleSortKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sorting = false
		m.updateViewportContent()
		return m, nil

	case "enter", " ":
		m.sortOrder = sortOptions[m.sortCursor]
		m.sorting = false
		m.clampCursorAndRefreshSearch()
		m.updateViewportContent()
		return m, nil

	case "up", "k":
		if m.sortCursor > 0 {
			m.sortCursor--
		}
		return m, nil

	case "down", "j":
		if m.sortCursor < len(sortOptions)-1 {
			m.sortCursor++
		}
		return m, nil
	}

	return m, nil
}

// clampCursorAndRefreshSearch clamps cursor to valid range after filter/sort change and re-runs search
func (m *Model) clampCursorAndRefreshSearch() {
	filtered := m.sortedResources()
	if m.cursor >= len(filtered) {
		if len(filtered) > 0 {
			m.cursor = len(filtered) - 1
		} else {
			m.cursor = 0
		}
	}
	if m.searchQuery != "" {
		m.performSearch()
	}
}

// expandAll expands all visible (filtered/sorted) resources
func (m *Model) expandAll() {
	for _, idx := range m.sortedResources() {
		m.expanded[idx] = true
	}
	m.updateViewportContent()
	m.ensureCursorVisible()
}

// collapseAll collapses all visible (filtered/sorted) resources
func (m *Model) collapseAll() {
	for _, idx := range m.sortedResources() {
		m.expanded[idx] = false
	}
	m.updateViewportContent()
	m.ensureCursorVisible()
}

// nextMatch moves to the next search match (searchMatches holds display indices)
func (m *Model) nextMatch() {
	if len(m.searchMatches) > 0 {
		m.currentMatch = (m.currentMatch + 1) % len(m.searchMatches)
		m.cursor = m.searchMatches[m.currentMatch]
		m.updateViewportContent()
		m.ensureCursorVisible()
	}
}

// prevMatch moves to the previous search match (searchMatches holds display indices)
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

// gotoBottom moves cursor to the last visible resource and scrolls so it's visible
func (m *Model) gotoBottom() {
	filtered := m.sortedResources()
	if len(filtered) > 0 {
		m.cursor = len(filtered) - 1
	}
	m.updateViewportContent()
	m.ensureCursorVisible()
	m.pendingG = false
}

func (m *Model) performSearch() {
	m.searchMatches = []int{}
	m.currentMatch = 0

	if m.searchQuery == "" {
		return
	}

	query := strings.ToLower(m.searchQuery)
	filtered := m.sortedResources()
	for displayIdx, resourceIdx := range filtered {
		r := m.plan.Resources[resourceIdx]
		if strings.Contains(strings.ToLower(r.Address), query) ||
			strings.Contains(strings.ToLower(r.Type), query) ||
			strings.Contains(strings.ToLower(r.Name), query) {
			m.searchMatches = append(m.searchMatches, displayIdx)
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

	if m.cursor < 0 || m.cursor >= len(m.resourceLineStarts) {
		return
	}

	lineNum := m.resourceLineStarts[m.cursor]

	topLine := m.viewport.YOffset
	bottomLine := topLine + m.viewport.Height - 1

	if lineNum < topLine {
		m.viewport.SetYOffset(lineNum)
	} else if lineNum > bottomLine {
		newOffset := lineNum - m.viewport.Height + 1
		if newOffset < 0 {
			newOffset = 0
		}
		m.viewport.SetYOffset(newOffset)
	}
}

// scrollForExpanded ensures the cursor is visible and, when expanded,
// positions the cursor near the top so the expanded content is visible below.
func (m *Model) scrollForExpanded() {
	if !m.ready || m.cursor < 0 || m.cursor >= len(m.resourceLineStarts) {
		return
	}

	lineNum := m.resourceLineStarts[m.cursor]
	filtered := m.sortedResources()
	resourceIdx := -1
	if m.cursor < len(filtered) {
		resourceIdx = filtered[m.cursor]
	}

	if resourceIdx >= 0 && m.expanded[resourceIdx] {
		var endLine int
		if m.cursor+1 < len(m.resourceLineStarts) {
			endLine = m.resourceLineStarts[m.cursor+1]
		} else {
			endLine = m.contentLineCount
		}

		bottomLine := m.viewport.YOffset + m.viewport.Height - 1
		if endLine > bottomLine {
			m.viewport.SetYOffset(lineNum)
			return
		}
	}

	m.ensureCursorVisible()
}

func (m *Model) renderResources() string {
	var b strings.Builder
	lineCount := 0

	filtered := m.sortedResources()
	m.resourceLineStarts = make([]int, len(filtered))

	if len(filtered) == 0 {
		b.WriteString(mutedColor.Render("No resources match the current filters. Press 'f' to change filters."))
		b.WriteString("\n")
		return b.String()
	}

	for displayIdx, resourceIdx := range filtered {
		m.resourceLineStarts[displayIdx] = lineCount
		r := m.plan.Resources[resourceIdx]

		isSelected := displayIdx == m.cursor
		isExpanded := m.expanded[resourceIdx]
		isMatch := false

		for _, match := range m.searchMatches {
			if match == displayIdx {
				isMatch = true
				break
			}
		}

		if isSelected {
			line := m.renderSelectedResourceLine(r, isExpanded, isMatch)
			b.WriteString(line)
		} else {
			line := m.renderResourceLine(r, isExpanded, isMatch)
			b.WriteString(line)
		}
		b.WriteString("\n")
		lineCount++

		if isExpanded && len(r.RawLines) > 1 {
			before := b.Len()
			m.renderExpandedContent(&b, r.RawLines[1:], r.Action)
			b.WriteString("\n")
			lineCount += strings.Count(b.String()[before:], "\n")
		}
	}

	m.contentLineCount = lineCount

	b.WriteString("\n")
	eolStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6c7086"))
	b.WriteString(eolStyle.Render("── End of Plan ──"))
	b.WriteString("\n")

	// Padding after the marker so the viewport has room to scroll
	// the last resource's expanded content fully into view
	for i := 0; i < m.viewport.Height; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

// renderExpandedContent renders the expanded lines for a resource, applying
// word wrapping, userdata decoding, and YAML/heredoc diff detection.
func (m Model) renderExpandedContent(b *strings.Builder, lines []string, action parser.Action) {
	maxWidth := m.viewport.Width

	for idx := 0; idx < len(lines); idx++ {
		line := lines[idx]

		if decoded, ok := m.tryRenderUserdata(line, action, maxWidth); ok {
			b.WriteString(decoded)
			b.WriteString("\n")
			continue
		}

		if consumed, rendered := m.tryRenderHeredocDiff(lines, idx, action, maxWidth); consumed > 0 {
			b.WriteString(rendered)
			idx += consumed - 1
			continue
		}

		coloredLine := m.wrapAndColorize(line, action, maxWidth)
		b.WriteString(coloredLine)
		b.WriteString("\n")
	}
}

// wrapAndColorize wraps a raw HCL line to the viewport width and colorizes
// each sub-line, preserving indentation and prefix alignment.
func (m Model) wrapAndColorize(line string, action parser.Action, maxWidth int) string {
	if maxWidth <= 0 {
		return m.colorizeHCLLine(line, action)
	}

	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	indentWidth := utf8.RuneCountInString(indent)

	var rawPrefix, content string
	lineAction := action
	switch {
	case strings.HasPrefix(trimmed, "+ "):
		rawPrefix = "+ "
		content = trimmed[2:]
		lineAction = parser.ActionCreate
	case strings.HasPrefix(trimmed, "- "):
		rawPrefix = "- "
		content = trimmed[2:]
		lineAction = parser.ActionDestroy
	case strings.HasPrefix(trimmed, "~ "):
		rawPrefix = "~ "
		content = trimmed[2:]
		lineAction = parser.ActionUpdate
	default:
		rawPrefix = "  "
		content = trimmed
	}

	prefixWidth := utf8.RuneCountInString(rawPrefix)
	availableWidth := maxWidth - indentWidth - prefixWidth
	if availableWidth < 20 || utf8.RuneCountInString(content) <= availableWidth {
		return m.colorizeHCLLine(line, action)
	}

	wrapped := wordwrap.String(content, availableWidth)
	subLines := strings.Split(wrapped, "\n")
	if len(subLines) <= 1 {
		return m.colorizeHCLLine(line, action)
	}

	continuationIndent := indent + strings.Repeat(" ", prefixWidth)

	var b strings.Builder
	for i, sub := range subLines {
		if i > 0 {
			b.WriteString("\n")
		}
		if i == 0 {
			reconstructed := indent + rawPrefix + sub
			b.WriteString(m.colorizeHCLLine(reconstructed, action))
		} else {
			b.WriteString(continuationIndent)
			b.WriteString(m.colorizeHCLContent(strings.TrimSpace(sub), lineAction))
		}
	}

	return b.String()
}

// tryRenderUserdata detects user_data attributes with base64 content and
// renders them decoded with diff highlighting for changes.
func (m Model) tryRenderUserdata(line string, action parser.Action, maxWidth int) (string, bool) {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	var rawPrefix string
	content := trimmed
	lineAction := action
	switch {
	case strings.HasPrefix(trimmed, "+ "):
		rawPrefix = "+ "
		content = trimmed[2:]
		lineAction = parser.ActionCreate
	case strings.HasPrefix(trimmed, "- "):
		rawPrefix = "- "
		content = trimmed[2:]
		lineAction = parser.ActionDestroy
	case strings.HasPrefix(trimmed, "~ "):
		rawPrefix = "~ "
		content = trimmed[2:]
		lineAction = parser.ActionUpdate
	default:
		rawPrefix = "  "
	}

	eqIdx := strings.Index(content, " = ")
	if eqIdx < 0 {
		return "", false
	}
	key := strings.TrimSpace(content[:eqIdx])
	if key != "user_data" && key != "user_data_base64" {
		return "", false
	}
	value := strings.TrimSpace(content[eqIdx+3:])

	var b strings.Builder
	decodedIndent := indent + strings.Repeat(" ", len(rawPrefix))
	headerLine := m.colorizeHCLLine(line, action)

	if strings.Contains(value, " -> ") {
		parts := strings.SplitN(value, " -> ", 2)
		oldB64 := unquote(strings.TrimSpace(parts[0]))
		newB64 := unquote(strings.TrimSpace(parts[1]))

		oldDecoded, oldOk := tryBase64Decode(oldB64)
		newDecoded, newOk := tryBase64Decode(newB64)

		if !oldOk && !newOk {
			return "", false
		}

		b.WriteString(headerLine)
		b.WriteString("\n")
		b.WriteString(decodedIndent)
		b.WriteString(mutedColor.Render("┄┄┄ decoded " + key + " ┄┄┄"))
		b.WriteString("\n")

		if oldOk && newOk {
			oldLines := strings.Split(oldDecoded, "\n")
			newLines := strings.Split(newDecoded, "\n")
			diff := ComputeDiff(oldLines, newLines)
			contextDiff := ContextDiff(diff, 3)
			if contextDiff == nil {
				b.WriteString(decodedIndent)
				b.WriteString(mutedColor.Render("  (no changes in decoded content)"))
				b.WriteString("\n")
			} else {
				renderDiffLines(&b, contextDiff, decodedIndent, maxWidth)
			}
		} else {
			if oldOk {
				for _, ol := range strings.Split(oldDecoded, "\n") {
					b.WriteString(decodedIndent)
					b.WriteString(lipgloss.NewStyle().Foreground(destroyColor).Render("- " + ol))
					b.WriteString("\n")
				}
			}
			if newOk {
				for _, nl := range strings.Split(newDecoded, "\n") {
					b.WriteString(decodedIndent)
					b.WriteString(lipgloss.NewStyle().Foreground(createColor).Render("+ " + nl))
					b.WriteString("\n")
				}
			}
		}

		b.WriteString(decodedIndent)
		b.WriteString(mutedColor.Render("┄┄┄ end " + key + " ┄┄┄"))
		return b.String(), true
	}

	raw := unquote(value)
	decoded, ok := tryBase64Decode(raw)
	if !ok {
		return "", false
	}

	b.WriteString(headerLine)
	b.WriteString("\n")
	b.WriteString(decodedIndent)
	b.WriteString(mutedColor.Render("┄┄┄ decoded " + key + " ┄┄┄"))
	b.WriteString("\n")

	style := lipgloss.NewStyle().Foreground(textColor)
	if lineAction == parser.ActionCreate {
		style = lipgloss.NewStyle().Foreground(createColor)
	} else if lineAction == parser.ActionDestroy {
		style = lipgloss.NewStyle().Foreground(destroyColor)
	}

	for _, dl := range strings.Split(decoded, "\n") {
		wrapped := wrapText(dl, maxWidth-len(decodedIndent)-2)
		for _, wl := range strings.Split(wrapped, "\n") {
			b.WriteString(decodedIndent)
			b.WriteString(style.Render("  " + wl))
			b.WriteString("\n")
		}
	}

	b.WriteString(decodedIndent)
	b.WriteString(mutedColor.Render("┄┄┄ end " + key + " ┄┄┄"))
	return b.String(), true
}

// tryRenderHeredocDiff detects paired remove/add heredoc blocks starting at
// index idx and renders them as a granular diff. Handles two patterns:
//   - Heredoc blocks: "- <<-EOT" ... "EOT," followed by "+ <<-EOT" ... "EOT,"
//   - Prefixed blocks: consecutive "- " lines followed by consecutive "+ " lines
func (m Model) tryRenderHeredocDiff(lines []string, idx int, action parser.Action, maxWidth int) (int, string) {
	if idx >= len(lines) {
		return 0, ""
	}

	trimmed := strings.TrimLeft(lines[idx], " \t")

	if strings.HasPrefix(trimmed, "- ") && isHeredocMarker(trimmed[2:]) {
		return m.renderHeredocPairDiff(lines, idx, maxWidth)
	}

	if strings.HasPrefix(trimmed, "- ") {
		return m.renderPrefixedBlockDiff(lines, idx, action, maxWidth)
	}

	return 0, ""
}

func isHeredocMarker(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "<<")
}

func parseHeredocEnd(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<<-")
	s = strings.TrimPrefix(s, "<<")
	return strings.TrimSpace(s)
}

// renderHeredocPairDiff handles paired heredoc blocks where content lines
// inside the heredoc do NOT have individual +/- prefixes.
func (m Model) renderHeredocPairDiff(lines []string, idx int, maxWidth int) (int, string) {
	firstTrimmed := strings.TrimLeft(lines[idx], " \t")
	endMarker := parseHeredocEnd(firstTrimmed[2:])
	if endMarker == "" {
		return 0, ""
	}

	oldStart := idx + 1
	oldEnd := oldStart
	for oldEnd < len(lines) {
		lt := strings.TrimSpace(lines[oldEnd])
		if lt == endMarker || lt == endMarker+"," {
			oldEnd++
			break
		}
		oldEnd++
	}

	if oldEnd >= len(lines) {
		return 0, ""
	}

	addHeredocIdx := oldEnd
	for addHeredocIdx < len(lines) {
		at := strings.TrimLeft(lines[addHeredocIdx], " \t")
		if strings.HasPrefix(at, "+ ") && isHeredocMarker(at[2:]) {
			break
		}
		if strings.TrimSpace(lines[addHeredocIdx]) == "" {
			addHeredocIdx++
			continue
		}
		return 0, ""
	}

	if addHeredocIdx >= len(lines) {
		return 0, ""
	}

	newStart := addHeredocIdx + 1
	newEnd := newStart
	for newEnd < len(lines) {
		lt := strings.TrimSpace(lines[newEnd])
		if lt == endMarker || lt == endMarker+"," {
			newEnd++
			break
		}
		newEnd++
	}

	oldContent := extractHeredocContent(lines[oldStart : oldEnd-1])
	newContent := extractHeredocContent(lines[newStart : newEnd-1])

	if len(oldContent) == 0 && len(newContent) == 0 {
		return 0, ""
	}

	diff := ComputeDiff(oldContent, newContent)
	contextDiff := ContextDiff(diff, 3)
	if contextDiff == nil {
		return 0, ""
	}

	baseIndent := extractIndent(lines[idx])

	var b strings.Builder
	b.WriteString(baseIndent)
	b.WriteString(mutedColor.Render("┄┄┄ heredoc diff ┄┄┄"))
	b.WriteString("\n")

	renderDiffLines(&b, contextDiff, baseIndent, maxWidth)

	b.WriteString(baseIndent)
	b.WriteString(mutedColor.Render("┄┄┄ end heredoc diff ┄┄┄"))
	b.WriteString("\n")

	return newEnd - idx, b.String()
}

// renderPrefixedBlockDiff handles blocks where each line has a +/- prefix.
func (m Model) renderPrefixedBlockDiff(lines []string, idx int, action parser.Action, maxWidth int) (int, string) {
	removeEnd := idx
	for removeEnd < len(lines) {
		t := strings.TrimLeft(lines[removeEnd], " \t")
		if !strings.HasPrefix(t, "- ") {
			break
		}
		removeEnd++
	}

	if removeEnd == idx {
		return 0, ""
	}

	addStart := removeEnd
	addEnd := removeEnd
	for addEnd < len(lines) {
		t := strings.TrimLeft(lines[addEnd], " \t")
		if !strings.HasPrefix(t, "+ ") {
			break
		}
		addEnd++
	}

	if addEnd == addStart {
		return 0, ""
	}

	if (removeEnd-idx) < 3 && (addEnd-addStart) < 3 {
		return 0, ""
	}

	oldContent := extractPrefixedContent(lines[idx:removeEnd], "- ")
	newContent := extractPrefixedContent(lines[addStart:addEnd], "+ ")

	if len(oldContent) == 0 || len(newContent) == 0 {
		return 0, ""
	}

	diff := ComputeDiff(oldContent, newContent)
	contextDiff := ContextDiff(diff, 3)
	if contextDiff == nil {
		return 0, ""
	}

	baseIndent := extractIndent(lines[idx])

	var b strings.Builder
	renderDiffLines(&b, contextDiff, baseIndent, maxWidth)

	return addEnd - idx, b.String()
}

// renderDiffLines writes context-diff lines into a builder, handling all
// DiffOp types including DiffSeparator for collapsed equal runs.
func renderDiffLines(b *strings.Builder, diff []DiffLine, indent string, maxWidth int) {
	for _, d := range diff {
		switch d.Op {
		case DiffSeparator:
			b.WriteString(indent)
			b.WriteString(mutedColor.Render("@@ ··· @@"))
			b.WriteString("\n")
		case DiffDelete:
			wrapped := wrapText(d.Text, maxWidth-len(indent)-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				b.WriteString(indent)
				b.WriteString(lipgloss.NewStyle().Foreground(destroyColor).Render("- " + wl))
				b.WriteString("\n")
			}
		case DiffInsert:
			wrapped := wrapText(d.Text, maxWidth-len(indent)-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				b.WriteString(indent)
				b.WriteString(lipgloss.NewStyle().Foreground(createColor).Render("+ " + wl))
				b.WriteString("\n")
			}
		case DiffEqual:
			wrapped := wrapText(d.Text, maxWidth-len(indent)-4)
			for _, wl := range strings.Split(wrapped, "\n") {
				b.WriteString(indent)
				b.WriteString(mutedColor.Render("  " + wl))
				b.WriteString("\n")
			}
		}
	}
}

func extractHeredocContent(lines []string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, strings.TrimRight(line, " \t"))
	}
	return result
}

func extractPrefixedContent(lines []string, prefix string) []string {
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, prefix) {
			result = append(result, trimmed[len(prefix):])
		}
	}
	return result
}

func extractIndent(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	return line[:len(line)-len(trimmed)]
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func tryBase64Decode(s string) (string, bool) {
	if s == "" || s == "null" || strings.HasPrefix(s, "(") {
		return "", false
	}
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(s)
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(s)
			if err != nil {
				return "", false
			}
		}
	}
	for _, b := range decoded {
		if b == 0 {
			return "", false
		}
	}
	return string(decoded), true
}

func wrapText(s string, width int) string {
	if width <= 10 {
		return s
	}
	return wordwrap.String(s, width)
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

// colorizeHCLLine applies syntax highlighting to a line of HCL in the TUI.
// The line-level prefix (+/-/~) drives content coloring instead of the
// resource-level action, so + lines are green and - lines are red even
// inside an "update" resource.
func (m Model) colorizeHCLLine(line string, action parser.Action) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	var prefix string
	var content string
	lineAction := action

	if strings.HasPrefix(trimmed, "+ ") {
		prefix = createSymbol
		content = trimmed[2:]
		lineAction = parser.ActionCreate
	} else if strings.HasPrefix(trimmed, "- ") {
		prefix = destroySymbol
		content = trimmed[2:]
		lineAction = parser.ActionDestroy
	} else if strings.HasPrefix(trimmed, "~ ") {
		prefix = updateSymbol
		content = trimmed[2:]
		lineAction = parser.ActionUpdate
	} else {
		prefix = " "
		content = trimmed
	}

	coloredContent := m.colorizeHCLContent(content, lineAction)

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

// sortOrderLabel returns a display label for a sort option
func sortOrderLabel(opt SortOrder) string {
	switch opt {
	case SortDefault:
		return "default (plan order)"
	case SortByAction:
		return "by action"
	case SortByAddress:
		return "by address"
	case SortByType:
		return "by type"
	default:
		return string(opt)
	}
}

// sortOrderHint returns a one-line hint explaining what a sort option does
func sortOrderHint(opt SortOrder) string {
	switch opt {
	case SortDefault:
		return "— as Terraform outputs them"
	case SortByAction:
		return "— group create, destroy, update, etc."
	case SortByAddress:
		return "— alphabetical by resource address"
	case SortByType:
		return "— group by resource type (aws_instance, etc.)"
	default:
		return ""
	}
}

// filterActionLabel returns a short label for the filter picker
func filterActionLabel(action parser.Action) string {
	switch action {
	case parser.ActionCreate:
		return "create"
	case parser.ActionDestroy:
		return "destroy"
	case parser.ActionUpdate:
		return "update"
	case parser.ActionReplace:
		return "replace"
	case parser.ActionRead:
		return "read"
	case parser.ActionDeleteCreate:
		return "destroy+create"
	case parser.ActionCreateDelete:
		return "create+destroy"
	default:
		return string(action)
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

	// Filter picker (when open)
	if m.filtering {
		b.WriteString(searchStyle.Render("Filter by status (Space: toggle, a: all, c: clear, Enter: apply, Esc: clear all and close)"))
		b.WriteString("\n\n")
		for i, action := range filterableActions {
			checked := "[ ]"
			if m.statusFilters != nil && m.statusFilters[action] {
				checked = "[x]"
			}
			label := filterActionLabel(action)
			rowStyle := lipgloss.NewStyle().Foreground(textColor)
			if i == m.filterCursor {
				rowStyle = rowStyle.Background(selectedBg)
			}
			// Color the label by action type
			labelStyle := GetResourceStyle(string(action))
			b.WriteString(rowStyle.Render("  "+checked+" ") + labelStyle.Render(label))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("j/k: navigate • Space: toggle • a: select all • c: clear all • Enter: apply • Esc: clear all and close"))
		return appStyle.Render(b.String())
	}

	// Sort picker (when open)
	if m.sorting {
		b.WriteString(searchStyle.Render("Sort by (Enter/Space: select, Esc: close)"))
		b.WriteString("\n\n")
		for i, opt := range sortOptions {
			marker := "  "
			if opt == m.sortOrder {
				marker = "● "
			}
			rowStyle := lipgloss.NewStyle().Foreground(textColor)
			if i == m.sortCursor {
				rowStyle = rowStyle.Background(selectedBg)
			}
			line := marker + sortOrderLabel(opt) + " " + mutedColor.Render(sortOrderHint(opt))
			b.WriteString(rowStyle.Render(line))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("j/k: navigate • Enter/Space: select • Esc: close"))
		return appStyle.Render(b.String())
	}

	// Filter status (when filters active, not in picker)
	if len(m.statusFilters) > 0 {
		var labels []string
		for _, action := range filterableActions {
			if m.statusFilters[action] {
				labels = append(labels, filterActionLabel(action))
			}
		}
		filterInfo := fmt.Sprintf("Filter: %s (%d active) • f: change • Esc: clear all", strings.Join(labels, ", "), len(labels))
		b.WriteString(searchStyle.Render(filterInfo))
		b.WriteString("\n\n")
	}

	// Sort status (when not default, not in picker)
	if m.sortOrder != SortDefault && m.sortOrder != "" {
		sortInfo := fmt.Sprintf("Sort: %s • s: change", sortOrderLabel(m.sortOrder))
		b.WriteString(searchStyle.Render(sortInfo))
		b.WriteString("\n\n")
	}

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
			help = fmt.Sprintf("%s • j/k/↑↓: navigate • e/c: all • /: search • f: filter • s: sort • q: quit", applyHint)
		}
	} else {
		help = "j/k/↑↓: navigate • l/→: expand • h/←/⌫: collapse • d/u: scroll • e/c: all • gg/G: top/bottom • /: search • f: filter • s: sort • q: quit"
		if len(m.statusFilters) > 0 {
			help += " • Esc: clear filter"
		}
	}
	b.WriteString(helpStyle.Render(help))

	return appStyle.Render(b.String())
}
