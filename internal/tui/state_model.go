package tui

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aymanbagabas/go-osc52/v2"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// StateSortOrder defines how state addresses are ordered
type StateSortOrder string

const (
	StateSortDefault     StateSortOrder = "default"
	StateSortByAddress   StateSortOrder = "address"
	StateSortByType      StateSortOrder = "type"
	StateSortByModuleDepth StateSortOrder = "moduleDepth"
)

var stateSortOptions = []StateSortOrder{
	StateSortDefault, StateSortByAddress, StateSortByType, StateSortByModuleDepth,
}

// ansiEscape matches common ANSI escape sequences (SGR, CSI, etc.)
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// copyFeedbackClearMsg clears the "Copied!" message after a delay
type copyFeedbackClearMsg struct{}

// flashMsgClearMsg clears the flash message after a delay
type flashMsgClearMsg struct{}

// stateShowResultMsg is sent when async state show completes (all addresses in parallel)
type stateShowResultMsg struct {
	results map[string]string // addr -> content
	errors  map[string]error  // addr -> err
}

// stateShowAllCmd runs terraform state show for all addresses in parallel
func stateShowAllCmd(tfCmd string, addrs []string, tfStateArgs []string) tea.Cmd {
	return func() tea.Msg {
		results := make(map[string]string)
		errors := make(map[string]error)
		type result struct {
			addr    string
			content string
			err     error
		}
		ch := make(chan result, len(addrs))
		for _, addr := range addrs {
			go func(addr string) {
				args := append([]string{"state", "show", addr}, tfStateArgs...)
				cmd := exec.Command(tfCmd, args...)
				output, err := cmd.CombinedOutput()
				ch <- result{addr, string(output), err}
			}(addr)
		}
		for range addrs {
			r := <-ch
			results[r.addr] = r.content
			if r.err != nil {
				errors[r.addr] = r.err
			}
		}
		return stateShowResultMsg{results: results, errors: errors}
	}
}

// stateAddressStyle returns a style for the address based on resource type
func stateAddressStyle(addr string) lipgloss.Style {
	typ := stateAddressType(addr)
	switch {
	case strings.HasPrefix(typ, "module"):
		return lipgloss.NewStyle().Foreground(replaceColor)
	case strings.HasPrefix(typ, "aws_"):
		return lipgloss.NewStyle().Foreground(headerColor)
	case strings.HasPrefix(typ, "google_"):
		return lipgloss.NewStyle().Foreground(createColor)
	case strings.HasPrefix(typ, "azurerm_"):
		return lipgloss.NewStyle().Foreground(readColor)
	default:
		return lipgloss.NewStyle().Foreground(textColor)
	}
}

// StateModel represents the TUI state for terraform state list/show/rm
type StateModel struct {
	addresses    []string
	tfCmd        string
	tfStateArgs  []string
	version      string
	cursor       int
	viewport     viewport.Model
	ready        bool
	width        int
	height       int
	searching    bool
	searchInput  textinput.Model
	searchQuery  string
	searchMatches []int
	currentMatch int

	// Sort
	sortOrder  StateSortOrder
	sorting    bool
	sortCursor int

	// Selection (keyed by address, persists across search/sort)
	selected          map[string]bool
	lastSelectedAddress string

	// Detail view
	showingDetail      bool
	detailContent      string
	detailViewport     viewport.Model
	detailLoading      bool
	detailAddresses    []string
	detailSearchQuery string
	detailSearchInput textinput.Model
	detailExpanded    map[int]bool
	detailBlocks      []string
	detailCopyFeedback string // "Copied!" shown briefly after y

	// Confirmation
	confirmMode   string // "", "rm", "taint", "untaint", "show"
	confirmTargets []string

	// Flash message (non-intrusive feedback, cleared after delay)
	flashMsg string

	// Tainted resources (addr -> true), updated on taint/untaint and parsed from state show
	tainted map[string]bool
}

// NewStateModel creates a new state TUI model
func NewStateModel(addresses []string, tfCmd string, tfStateArgs []string, version string) StateModel {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100
	ti.Width = 40

	detailTi := textinput.New()
	detailTi.Placeholder = "Search in content..."
	detailTi.CharLimit = 100
	detailTi.Width = 40

	return StateModel{
		addresses:         addresses,
		tfCmd:             tfCmd,
		tfStateArgs:       tfStateArgs,
		version:           version,
		searchInput:       ti,
		detailSearchInput: detailTi,
		searchMatches:     []int{},
		sortOrder:         StateSortDefault,
		selected:          make(map[string]bool),
		detailExpanded:    make(map[int]bool),
		tainted:           make(map[string]bool),
	}
}

func (m *StateModel) sortedAddresses() []string {
	indices := make([]int, len(m.addresses))
	for i := range m.addresses {
		indices[i] = i
	}
	if m.sortOrder == StateSortDefault || m.sortOrder == "" {
		return m.addresses
	}
	sort.Slice(indices, func(i, j int) bool {
		a, b := m.addresses[indices[i]], m.addresses[indices[j]]
		switch m.sortOrder {
		case StateSortByAddress:
			return a < b
		case StateSortByType:
			ta, tb := stateAddressType(a), stateAddressType(b)
			if ta != tb {
				return ta < tb
			}
			return a < b
		case StateSortByModuleDepth:
			da, db := stateModuleDepth(a), stateModuleDepth(b)
			if da != db {
				return da < db
			}
			return a < b
		}
		return false
	})
	result := make([]string, len(indices))
	for i, idx := range indices {
		result[i] = m.addresses[idx]
	}
	return result
}

func stateAddressType(addr string) string {
	parts := strings.Split(addr, ".")
	if len(parts) >= 2 {
		return parts[len(parts)-2]
	}
	return addr
}

func stateModuleDepth(addr string) int {
	return strings.Count(addr, ".")
}

func (m *StateModel) displayedAddresses() []string {
	sorted := m.sortedAddresses()
	if m.searchQuery == "" {
		return sorted
	}
	if len(m.searchMatches) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(m.searchMatches))
	for _, idx := range m.searchMatches {
		if idx >= 0 && idx < len(sorted) {
			result = append(result, sorted[idx])
		}
	}
	return result
}

func (m *StateModel) performSearch() {
	m.searchMatches = []int{}
	m.currentMatch = 0
	if m.searchQuery == "" {
		return
	}
	terms := strings.Fields(strings.ToLower(m.searchQuery))
	if len(terms) == 0 {
		return
	}
	sorted := m.sortedAddresses()
	for i, addr := range sorted {
		searchable := strings.ToLower(addr)
		allMatch := true
		for _, term := range terms {
			if !strings.Contains(searchable, term) {
				allMatch = false
				break
			}
		}
		if allMatch {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
	if len(m.searchMatches) > 0 {
		m.cursor = 0
		m.currentMatch = 0
	}
}

func (m *StateModel) clampCursor() {
	displayed := m.displayedAddresses()
	if m.cursor >= len(displayed) {
		if len(displayed) > 0 {
			m.cursor = len(displayed) - 1
		} else {
			m.cursor = 0
		}
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

// syncViewportToCursor scrolls the list viewport so the cursor stays visible
func (m *StateModel) syncViewportToCursor() {
	displayed := m.displayedAddresses()
	if len(displayed) == 0 {
		return
	}
	h := m.viewport.Height - m.viewport.Style.GetVerticalFrameSize()
	if h <= 0 {
		h = m.viewport.Height
	}
	if h <= 0 {
		h = 1
	}
	maxOffset := len(displayed) - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.cursor < m.viewport.YOffset {
		m.viewport.SetYOffset(m.cursor)
	} else if m.cursor >= m.viewport.YOffset+h {
		m.viewport.SetYOffset(m.cursor - h + 1)
	}
	if m.viewport.YOffset > maxOffset {
		m.viewport.SetYOffset(maxOffset)
	}
}

func (m *StateModel) getSelectedAddresses() []string {
	displayed := m.displayedAddresses()
	var out []string
	for addr := range m.selected {
		for _, d := range displayed {
			if d == addr {
				out = append(out, addr)
				break
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	if len(displayed) > 0 && m.cursor >= 0 && m.cursor < len(displayed) {
		return []string{displayed[m.cursor]}
	}
	return nil
}

// runStateRm runs terraform state rm and returns (output, error). Does not print to terminal.
func (m *StateModel) runStateRm(addrs []string) (string, error) {
	args := append([]string{"state", "rm"}, m.tfStateArgs...)
	args = append(args, addrs...)
	cmd := exec.Command(m.tfCmd, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// runTaint runs terraform taint and returns (output, error). Does not print to terminal.
func (m *StateModel) runTaint(addr string) (string, error) {
	cmd := exec.Command(m.tfCmd, "taint", addr)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// runUntaint runs terraform untaint and returns (output, error). Does not print to terminal.
func (m *StateModel) runUntaint(addr string) (string, error) {
	cmd := exec.Command(m.tfCmd, "untaint", addr)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (m *StateModel) refreshAddresses() {
	args := append([]string{"state", "list"}, m.tfStateArgs...)
	cmd := exec.Command(m.tfCmd, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return
	}
	lines := strings.Split(string(output), "\n")
	var addrs []string
	for _, line := range lines {
		addr := strings.TrimSpace(line)
		if addr != "" {
			addrs = append(addrs, addr)
		}
	}
	m.addresses = addrs
	// Clear selection for addresses that no longer exist
	for addr := range m.selected {
		found := false
		for _, a := range addrs {
			if a == addr {
				found = true
				break
			}
		}
		if !found {
			delete(m.selected, addr)
		}
	}
}

func (m StateModel) Init() tea.Cmd {
	return nil
}

func (m StateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case copyFeedbackClearMsg:
		m.detailCopyFeedback = ""
		return m, nil
	case flashMsgClearMsg:
		m.flashMsg = ""
		return m, nil
	case stateShowResultMsg:
		var sb strings.Builder
		for _, addr := range m.detailAddresses {
			if err, ok := msg.errors[addr]; ok {
				sb.WriteString(fmt.Sprintf("# %s (error: %v)\n%s\n\n", addr, err, msg.results[addr]))
			} else {
				sb.WriteString(fmt.Sprintf("# %s\n%s\n\n", addr, msg.results[addr]))
			}
		}
		m.detailContent = sb.String()
		m.detailLoading = false
		m.detailBlocks = parseDetailBlocks(m.detailContent)
		for i := range m.detailBlocks {
			m.detailExpanded[i] = true
		}
		// Parse tainted status from block headers (e.g. "# addr: (tainted)")
		for _, block := range m.detailBlocks {
			if addr := parseBlockAddress(block); addr != "" && strings.Contains(block, "(tainted)") {
				m.tainted[addr] = true
			}
		}
		w, h := m.width-4, m.height-8
		if w < 20 {
			w = 20
		}
		if h < 10 {
			h = 10
		}
		if m.detailViewport.Height == 0 {
			m.detailViewport = viewport.New(w, h)
		} else {
			m.detailViewport.Width = w
			m.detailViewport.Height = h
		}
		m.detailViewport.SetContent(m.renderDetailContent())
		m.detailViewport.GotoTop()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 4
		footerHeight := 4
		if !m.ready {
			m.viewport = viewport.New(msg.Width-4, msg.Height-headerHeight-footerHeight)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width - 4
			m.viewport.Height = msg.Height - headerHeight - footerHeight
		}
		if m.showingDetail {
			m.detailViewport.Width = msg.Width - 4
			m.detailViewport.Height = msg.Height - headerHeight - footerHeight
		}
		m.viewport.SetContent(m.renderList())
		m.syncViewportToCursor()

	case tea.KeyMsg:
		if m.showingDetail {
			return m.handleDetailKey(msg)
		}
		if m.confirmMode != "" {
			return m.handleConfirmKey(msg)
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
				m.clampCursor()
				m.viewport.SetContent(m.renderList())
				m.syncViewportToCursor()
			case "esc":
				m.searching = false
				m.searchInput.SetValue("")
				m.searchQuery = ""
				m.searchMatches = []int{}
				m.clampCursor()
				m.viewport.SetContent(m.renderList())
				m.syncViewportToCursor()
			case "up":
				if m.cursor > 0 {
					m.cursor--
					m.viewport.SetContent(m.renderList())
					m.syncViewportToCursor()
				}
			case "down":
				displayed := m.displayedAddresses()
				if m.cursor < len(displayed)-1 {
					m.cursor++
					m.viewport.SetContent(m.renderList())
					m.syncViewportToCursor()
				}
			default:
				m.searchInput, cmd = m.searchInput.Update(msg)
				m.searchQuery = m.searchInput.Value()
				m.performSearch()
				m.clampCursor()
				m.viewport.SetContent(m.renderList())
				m.syncViewportToCursor()
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}
		return m.handleListKey(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m *StateModel) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	displayed := m.displayedAddresses()

	// Ctrl+Space for range select (use key.Matches - terminal-dependent)
		if key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+space", "ctrl+ ", "ctrl+@"))) {
		if len(displayed) > 0 && m.cursor >= 0 && m.cursor < len(displayed) {
			anchorIdx := -1
			for i, a := range displayed {
				if a == m.lastSelectedAddress {
					anchorIdx = i
					break
				}
			}
			if anchorIdx < 0 {
				anchorIdx = m.cursor
				m.lastSelectedAddress = displayed[m.cursor]
			}
			minIdx, maxIdx := anchorIdx, m.cursor
			if minIdx > maxIdx {
				minIdx, maxIdx = maxIdx, minIdx
			}
			for i := minIdx; i <= maxIdx; i++ {
				m.selected[displayed[i]] = true
			}
			m.viewport.SetContent(m.renderList())
			m.syncViewportToCursor()
		}
		return m, nil
	}

	key := msg.String()
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.selected = make(map[string]bool)
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.viewport.SetContent(m.renderList())
			m.syncViewportToCursor()
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(displayed)-1 {
			m.cursor++
			m.viewport.SetContent(m.renderList())
			m.syncViewportToCursor()
		}
		return m, nil
	case " ":
		if len(displayed) > 0 && m.cursor >= 0 && m.cursor < len(displayed) {
			addr := displayed[m.cursor]
			m.selected[addr] = !m.selected[addr]
			m.lastSelectedAddress = addr
			m.viewport.SetContent(m.renderList())
		}
		return m, nil
	case "enter":
		targets := m.getSelectedAddresses()
		if len(targets) == 0 {
			return m, nil
		}
		m.detailAddresses = targets
		m.detailLoading = true
		m.showingDetail = true
		m.detailContent = ""
		m.detailBlocks = nil
		m.detailExpanded = make(map[int]bool)
		// Run all state show in parallel
		return m, stateShowAllCmd(m.tfCmd, targets, m.tfStateArgs)
	case "t":
		targets := m.getSelectedAddresses()
		if len(targets) == 0 {
			return m, nil
		}
		m.confirmMode = "taint"
		m.confirmTargets = targets
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "u":
		targets := m.getSelectedAddresses()
		if len(targets) == 0 {
			return m, nil
		}
		m.confirmMode = "untaint"
		m.confirmTargets = targets
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "d", "x":
		targets := m.getSelectedAddresses()
		if len(targets) == 0 {
			return m, nil
		}
		m.confirmMode = "rm"
		m.confirmTargets = targets
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "/":
		m.searching = true
		m.searchInput.Focus()
		return m, textinput.Blink
	case "f", "s":
		m.sorting = true
		m.sortCursor = 0
		for i, opt := range stateSortOptions {
			if opt == m.sortOrder {
				m.sortCursor = i
				break
			}
		}
		return m, nil
	case "g":
		m.cursor = 0
		m.viewport.GotoTop()
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "G":
		if len(displayed) > 0 {
			m.cursor = len(displayed) - 1
			m.viewport.SetContent(m.renderList())
			m.syncViewportToCursor()
		}
		return m, nil
	case "pgdown":
		h := m.viewport.Height - m.viewport.Style.GetVerticalFrameSize()
		if h <= 0 {
			h = m.viewport.Height
		}
		if h <= 0 {
			h = 1
		}
		if m.cursor+h < len(displayed) {
			m.cursor += h
		} else {
			m.cursor = len(displayed) - 1
		}
		m.viewport.SetContent(m.renderList())
		m.syncViewportToCursor()
		return m, nil
	case "pgup", "b":
		h := m.viewport.Height - m.viewport.Style.GetVerticalFrameSize()
		if h <= 0 {
			h = m.viewport.Height
		}
		if h <= 0 {
			h = 1
		}
		if m.cursor > h {
			m.cursor -= h
		} else {
			m.cursor = 0
		}
		m.viewport.SetContent(m.renderList())
		m.syncViewportToCursor()
		return m, nil
	}

	return m, nil
}

func (m *StateModel) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "esc" {
		m.confirmMode = ""
		m.confirmTargets = nil
		m.viewport.SetContent(m.renderList())
		m.syncViewportToCursor()
		return m, nil
	}
	if key == "y" {
		var cmd tea.Cmd
		switch m.confirmMode {
		case "rm":
			out, err := m.runStateRm(m.confirmTargets)
			if err != nil {
				// Use terraform output if available, else error
				msg := strings.TrimSpace(out)
				if msg == "" {
					msg = err.Error()
				}
				m.flashMsg = "Error: " + msg
			} else {
				for _, addr := range m.confirmTargets {
					delete(m.selected, addr)
					delete(m.tainted, addr)
				}
				m.refreshAddresses()
				m.flashMsg = fmt.Sprintf("Removed %d resource(s) from state", len(m.confirmTargets))
			}
		case "taint":
			for _, addr := range m.confirmTargets {
				_, err := m.runTaint(addr)
				if err == nil {
					m.tainted[addr] = true
				}
			}
			m.refreshAddresses()
			m.flashMsg = fmt.Sprintf("Tainted %d resource(s)", len(m.confirmTargets))
		case "untaint":
			for _, addr := range m.confirmTargets {
				_, err := m.runUntaint(addr)
				if err == nil {
					delete(m.tainted, addr)
				}
			}
			m.refreshAddresses()
			m.flashMsg = fmt.Sprintf("Untainted %d resource(s)", len(m.confirmTargets))
		}
		m.confirmMode = ""
		m.confirmTargets = nil
		m.viewport.SetContent(m.renderList())
		m.syncViewportToCursor()
		if m.flashMsg != "" {
			cmd = tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return flashMsgClearMsg{} })
		}
		return m, cmd
	}
	// Any other key cancels
	m.confirmMode = ""
	m.confirmTargets = nil
	m.viewport.SetContent(m.renderList())
	m.syncViewportToCursor()
	return m, nil
}

func (m *StateModel) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.detailSearchInput.Focused() {
		switch msg.String() {
		case "esc":
			m.detailSearchInput.Blur()
			m.detailSearchQuery = ""
			m.detailViewport.SetContent(m.renderDetailContent())
			return m, nil
		case "enter":
			m.detailSearchInput.Blur()
			m.detailSearchQuery = m.detailSearchInput.Value()
			m.detailViewport.SetContent(m.renderDetailContent())
			return m, nil
		default:
			var cmd tea.Cmd
			m.detailSearchInput, cmd = m.detailSearchInput.Update(msg)
			m.detailSearchQuery = m.detailSearchInput.Value()
			m.detailViewport.SetContent(m.renderDetailContent())
			return m, cmd
		}
	}
	switch msg.String() {
	case "esc", "b":
		m.showingDetail = false
		m.detailContent = ""
		m.detailBlocks = nil
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "/":
		m.detailSearchInput.Focus()
		return m, textinput.Blink
	case "e":
		for i := range m.detailBlocks {
			m.detailExpanded[i] = true
		}
		m.detailViewport.SetContent(m.renderDetailContent())
		return m, nil
	case "c":
		for i := range m.detailBlocks {
			m.detailExpanded[i] = false
		}
		m.detailViewport.SetContent(m.renderDetailContent())
		return m, nil
	case "j", "down":
		m.detailViewport.LineDown(1)
		return m, nil
	case "k", "up":
		m.detailViewport.LineUp(1)
		return m, nil
	case "pgdown", " ":
		m.detailViewport.PageDown()
		return m, nil
	case "pgup":
		m.detailViewport.PageUp()
		return m, nil
	case "y":
		if m.detailContent != "" {
			m.detailCopyFeedback = "Copied!"
			seq := osc52.New(m.detailContent)
			if os.Getenv("TMUX") != "" {
				seq = seq.Tmux()
			}
			fmt.Fprint(os.Stderr, seq)
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return copyFeedbackClearMsg{} })
		}
		return m, nil
	case "Y":
		content := m.renderDetailContent()
		lines := strings.Split(content, "\n")
		idx := m.detailViewport.YOffset
		if idx >= 0 && idx < len(lines) {
			line := stripANSI(strings.TrimRight(lines[idx], "\r"))
			if line != "" {
				m.detailCopyFeedback = "Copied line!"
				seq := osc52.New(line)
				if os.Getenv("TMUX") != "" {
					seq = seq.Tmux()
				}
				fmt.Fprint(os.Stderr, seq)
				return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return copyFeedbackClearMsg{} })
			}
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.detailViewport, cmd = m.detailViewport.Update(msg)
		return m, cmd
	}
}

func (m *StateModel) handleSortKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sorting = false
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "enter", " ":
		m.sortOrder = stateSortOptions[m.sortCursor]
		m.sorting = false
		m.performSearch()
		m.clampCursor()
		m.viewport.SetContent(m.renderList())
		return m, nil
	case "up", "k":
		if m.sortCursor > 0 {
			m.sortCursor--
		}
		return m, nil
	case "down", "j":
		if m.sortCursor < len(stateSortOptions)-1 {
			m.sortCursor++
		}
		return m, nil
	}
	return m, nil
}

// parseBlockAddress extracts the resource address from a block header (first line).
// Handles "# addr", "# addr: (tainted)", "# addr (error: ...)".
func parseBlockAddress(block string) string {
	idx := strings.Index(block, "\n")
	header := block
	if idx > 0 {
		header = block[:idx]
	}
	header = strings.TrimPrefix(strings.TrimSpace(header), "# ")
	for _, sep := range []string{": (tainted)", ": (error:", " (error:"} {
		if i := strings.Index(header, sep); i > 0 {
			return strings.TrimSpace(header[:i])
		}
	}
	if i := strings.Index(header, ": "); i > 0 {
		return strings.TrimSpace(header[:i])
	}
	return strings.TrimSpace(header)
}

// parseDetailBlocks splits state show output into collapsible blocks (split on # header lines)
func parseDetailBlocks(content string) []string {
	var blocks []string
	parts := strings.Split(content, "\n# ")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if i > 0 {
			p = "# " + p
		}
		blocks = append(blocks, p)
	}
	return blocks
}

func (m *StateModel) visibleDetailBlockIndices() []int {
	query := strings.ToLower(strings.TrimSpace(m.detailSearchQuery))
	var indices []int
	for i, block := range m.detailBlocks {
		if query == "" || strings.Contains(strings.ToLower(block), query) {
			indices = append(indices, i)
		}
	}
	return indices
}

// detailLinesForBlock returns the lines to show for a block; when searching, filters to matching lines (header always shown)
func (m *StateModel) detailLinesForBlock(block string, blockIdx int, query string) []string {
	lines := strings.Split(block, "\n")
	if query == "" || len(lines) == 0 {
		return lines
	}
	q := strings.ToLower(query)
	// Always include header (first line)
	out := []string{lines[0]}
	for _, line := range lines[1:] {
		if strings.Contains(strings.ToLower(line), q) {
			out = append(out, line)
		}
	}
	if len(out) == 1 {
		return lines // show all if no body line matches
	}
	return out
}

func (m *StateModel) renderDetailContent() string {
	var b strings.Builder
	query := strings.ToLower(strings.TrimSpace(m.detailSearchQuery))
	visible := m.visibleDetailBlockIndices()
	for _, bi := range visible {
		block := m.detailBlocks[bi]
		expanded := m.detailExpanded[bi]
		lines := m.detailLinesForBlock(block, bi, query)
		header := ""
		if len(lines) > 0 {
			header = lines[0]
		}
		symbol := "▼ "
		if !expanded {
			symbol = "▶ "
		}
		taintedBadge := ""
		if strings.Contains(block, "(tainted)") {
			taintedBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Bold(true).Render("⊗ ") 
		}
		headerStyle := lipgloss.NewStyle().Foreground(headerColor).Bold(true)
		b.WriteString(mutedColor.Render(symbol))
		b.WriteString(taintedBadge)
		b.WriteString(headerStyle.Render(header))
		b.WriteString("\n")
		if expanded && len(lines) > 1 {
			for _, line := range lines[1:] {
				colored := colorizeStateShowLine(line)
				b.WriteString(colored)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func colorizeStateShowLine(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	switch {
	case strings.HasPrefix(trimmed, "="):
		return indent + mutedColor.Render(trimmed)
	case strings.Contains(trimmed, " = "):
		idx := strings.Index(trimmed, " = ")
		name := trimmed[:idx]
		value := trimmed[idx+3:]
		return indent + attrNameStyle.Render(name) + " = " + lipgloss.NewStyle().Foreground(createColor).Render(value)
	default:
		return indent + lipgloss.NewStyle().Foreground(textColor).Render(trimmed)
	}
}

func stateSortLabel(opt StateSortOrder) string {
	switch opt {
	case StateSortDefault:
		return "default (list order)"
	case StateSortByAddress:
		return "by address"
	case StateSortByType:
		return "by type"
	case StateSortByModuleDepth:
		return "by module depth"
	default:
		return string(opt)
	}
}

func (m *StateModel) renderList() string {
	var b strings.Builder
	displayed := m.displayedAddresses()
	if len(displayed) == 0 {
		if m.searchQuery != "" {
			b.WriteString(mutedColor.Render("  No resources match search"))
		} else {
			b.WriteString(mutedColor.Render("  No resources in state"))
		}
		b.WriteString("\n")
		return b.String()
	}
	for i, addr := range displayed {
		marker := "  "
		if m.selected[addr] {
			marker = "● "
		}
		taintBadge := ""
		if m.tainted[addr] {
			taintBadge = lipgloss.NewStyle().Foreground(lipgloss.Color("#f9e2af")).Bold(true).Render(" ⊗")
		}
		addrStyle := stateAddressStyle(addr)
		line := marker + addr + taintBadge
		if i == m.cursor {
			addrStyle = addrStyle.Background(selectedBg).Bold(true)
		}
		b.WriteString(addrStyle.Render(line))
		b.WriteString("\n")
	}
	return b.String()
}

func (m StateModel) View() string {
	if !m.ready {
		return "Loading..."
	}
	if m.showingDetail {
		return m.viewDetail()
	}
	if m.confirmMode != "" {
		return m.viewWithConfirmation()
	}
	if m.sorting {
		return m.viewSortPicker()
	}

	var b strings.Builder
	b.WriteString(headerStyle.Render("🔺 Terra-Prism - State Viewer"))
	b.WriteString("\n")
	b.WriteString(summaryStyle.Render(fmt.Sprintf("  %d resources in state", len(m.addresses))))
	b.WriteString("\n\n")

	if m.searching {
		b.WriteString(searchStyle.Render("Search: "))
		b.WriteString(m.searchInput.View())
		b.WriteString("\n\n")
	} else if m.searchQuery != "" {
		b.WriteString(searchStyle.Render(fmt.Sprintf("Search: %q (%d/%d)", m.searchQuery, m.currentMatch+1, len(m.searchMatches))))
		b.WriteString("\n\n")
	}

	if m.sortOrder != StateSortDefault && m.sortOrder != "" {
		b.WriteString(searchStyle.Render(fmt.Sprintf("Sort: %s • f: change", stateSortLabel(m.sortOrder))))
		b.WriteString("\n\n")
	}

	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	help := "j/k: navigate • g/G: first/last • pgup/pgdn: page • Space: select • Ctrl+Space: range • Enter: show • t: taint • u: untaint • d/x: rm • /: search • f: sort • Esc: clear • q: quit"
	if m.flashMsg != "" {
		flashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
		help = flashStyle.Render("✓ "+m.flashMsg) + "  " + helpStyle.Render(help)
	} else {
		help = helpStyle.Render(help)
	}
	b.WriteString(help)
	return appStyle.Render(b.String())
}

func (m StateModel) viewDetail() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("🔺 Terra-Prism - State Details"))
	b.WriteString("\n")
	if m.detailLoading {
		b.WriteString(mutedColor.Render("  Loading..."))
		b.WriteString("\n\n")
	} else {
		if m.detailSearchInput.Focused() {
			b.WriteString(searchStyle.Render("Search: "))
			b.WriteString(m.detailSearchInput.View())
			b.WriteString("\n\n")
		} else if m.detailSearchQuery != "" {
			b.WriteString(searchStyle.Render(fmt.Sprintf("Search: %q", m.detailSearchQuery)))
			b.WriteString("\n\n")
		}
		b.WriteString(m.detailViewport.View())
	}
	b.WriteString("\n")
	help := "Esc/b: back • /: search • e/c: expand/collapse • j/k: scroll • y: copy all • Y: copy line"
	if m.detailCopyFeedback != "" {
		help = m.detailCopyFeedback + " • " + help
	}
	if m.flashMsg != "" {
		flashStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#a6e3a1")).Bold(true)
		help = flashStyle.Render("✓ "+m.flashMsg) + "  " + help
	}
	b.WriteString(helpStyle.Render(help))
	return appStyle.Render(b.String())
}

func (m StateModel) viewWithConfirmation() string {
	var msg string
	switch m.confirmMode {
	case "rm":
		msg = fmt.Sprintf("Remove %d resource(s) from state? (y/N)", len(m.confirmTargets))
	case "taint":
		msg = fmt.Sprintf("Taint %d resource(s)? (y/N)", len(m.confirmTargets))
	case "untaint":
		msg = fmt.Sprintf("Untaint %d resource(s)? (y/N)", len(m.confirmTargets))
	default:
		msg = "Confirm? (y/N)"
	}
	confirmStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#45475a")).
		Foreground(lipgloss.Color("#cdd6f4")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#f38ba8")).
		Padding(0, 2)
	var b strings.Builder
	b.WriteString(headerStyle.Render("🔺 Terra-Prism - State Viewer"))
	b.WriteString("\n")
	b.WriteString(summaryStyle.Render(fmt.Sprintf("  %d resources in state", len(m.addresses))))
	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")
	b.WriteString(confirmStyle.Render("⚠ "+msg+"  y: confirm  Esc: cancel"))
	return appStyle.Render(b.String())
}

func (m StateModel) viewSortPicker() string {
	var b strings.Builder
	b.WriteString(searchStyle.Render("Sort by (Enter/Space: select, Esc: close)"))
	b.WriteString("\n\n")
	for i, opt := range stateSortOptions {
		marker := "  "
		if opt == m.sortOrder {
			marker = "● "
		}
		rowStyle := lipgloss.NewStyle().Foreground(textColor)
		if i == m.sortCursor {
			rowStyle = rowStyle.Background(selectedBg)
		}
		b.WriteString(rowStyle.Render(marker + stateSortLabel(opt)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("j/k: navigate • Enter/Space: select • Esc: close"))
	return appStyle.Render(b.String())
}
