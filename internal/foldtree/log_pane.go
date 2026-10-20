package foldtree

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
	"github.com/muesli/reflow/wrap"
)

// wrapMinWidth is the narrowest width LogPane will actually word-wrap at;
// below it, wordwrap.String tends to produce a near-unreadable
// one-character-per-line result, so lines are left as-is (relying on
// horizontal scroll instead) rather than wrapped into garbage.
const wrapMinWidth = 10

// LogPane is an append-only, auto-scrolling text pane for showing a
// running (or already-finished) process's output — e.g. `terraform plan`
// or `terraform apply` output. It stays pinned to the bottom as new
// content arrives unless the user has manually scrolled up to inspect
// history, exactly like a terminal's own scrollback/tail behavior.
//
// Lines can be word-wrapped to the pane's width ('w' toggles it, off by
// default so raw output looks exactly like a real terminal until asked
// otherwise); horizontal scroll (bubbles/viewport's own default keymap,
// left/right or h/l) is how the unwrapped view — or any line wordwrap
// can't shrink even when wrapping is on, like a single unbroken token
// wider than the pane — gets inspected. An active search highlights
// every occurrence of the query within the visible text, not just the
// line it's on.
//
// LogPane is deliberately not built on Node/State: log lines are a flat,
// ever-growing list, not a collapsible tree, so it wraps bubbles/viewport
// directly instead of forcing an unrelated shape through the fold-tree
// machinery. It does supply its own g/G (top/bottom) and "/"-search,
// mirroring TreeView's own key vocabulary — bubbles/viewport's own
// default keymap already covers j/k/u/d/pgup/pgdown and left/right.
type LogPane struct {
	viewport    viewport.Model
	lines       []string // original, unwrapped lines -- what search matches against
	visible     bool
	ready       bool
	pendingG    bool
	title       string // "" hides the title bar entirely (the default)
	totalHeight int    // height last passed to SetSize; viewport.Height is derived from this minus the title bar's own line, if any

	// wrappedLineStarts[i] is the row index within the viewport's
	// displayed content where original line i begins -- needed to scroll
	// a match into view correctly, since wrapping (when on) can turn one
	// logical line into several displayed rows.
	wrappedLineStarts []int
	wordWrap          bool // off by default; toggled with 'w'

	searching   bool
	searchInput textinput.Model
	searchQuery string
	matches     []int // line indices (into lines) matching searchQuery
	matchPos    int   // index into matches of the current one
}

var _ tea.Model = LogPane{}

// horizontalScrollStep is how many columns left/right move per keypress.
// bubbles/viewport disables horizontal scrolling by default (its own
// horizontalStep is 0 unless set), which would otherwise make h/left and
// l/right silent no-ops on any line wordwrap couldn't shrink to width.
const horizontalScrollStep = 4

// NewLogPane returns an empty, hidden LogPane. Call SetSize before it's
// shown for the first time.
func NewLogPane() *LogPane {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 200
	ti.Width = 40
	p := &LogPane{searchInput: ti}
	p.viewport.SetHorizontalStep(horizontalScrollStep)
	return p
}

// SetSize resizes the pane, preserving whether it was pinned to the
// bottom. A width change re-wraps existing content. height is the pane's
// total on-screen height, including the title bar's own line when a
// title is set (see SetTitle) -- the caller's height budget for this
// pane never changes because of a title, only how much of it the
// viewport itself gets.
func (p *LogPane) SetSize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	wasAtBottom := !p.ready || p.viewport.AtBottom()
	widthChanged := p.viewport.Width != width
	p.viewport.Width = width
	p.totalHeight = height
	p.viewport.Height = p.contentHeight()
	p.ready = true
	if widthChanged {
		p.refreshContent()
	}
	if wasAtBottom {
		p.viewport.GotoBottom()
	}
}

// SetTitle sets the text shown in the pane's title bar: a single
// horizontal rule with the title embedded near its left edge, spanning
// only the pane's top edge -- not a full box border -- so a host can
// tell the pane apart from whatever's stacked above it without the
// visual weight of a frame. "" (the default) hides the title bar
// entirely and gives its line back to the viewport.
func (p *LogPane) SetTitle(title string) {
	if p.title == title {
		return
	}
	p.title = title
	p.viewport.Height = p.contentHeight()
}

// contentHeight returns how much of totalHeight the viewport itself
// gets, reserving one line for the title bar when one is set.
func (p LogPane) contentHeight() int {
	h := p.totalHeight
	if p.title != "" {
		h--
	}
	if h < 0 {
		h = 0
	}
	return h
}

// SetLines bulk-loads a completed document (e.g. `terraform plan`'s
// already-captured output) and resets scroll to the top, since this is
// a document to read, not a stream to follow. Clears any active search,
// since it was matching a now-replaced document.
func (p *LogPane) SetLines(lines []string) {
	p.lines = append([]string(nil), lines...)
	p.clearSearch() // also rebuilds wrapped content
	p.viewport.GotoTop()
}

// Append adds one streamed line. If the pane was already scrolled to the
// bottom, it stays pinned there; if the user had scrolled up to inspect
// earlier output, their position is preserved. An active search's
// matches are not recomputed against the new line until the user
// re-searches -- search reflects a point in time, not a live filter.
func (p *LogPane) Append(line string) {
	wasAtBottom := p.viewport.AtBottom()
	p.lines = append(p.lines, line)
	p.refreshContent()
	if wasAtBottom {
		p.viewport.GotoBottom()
	}
}

// Reset clears all content and scroll position — call before a new run's
// output starts arriving.
func (p *LogPane) Reset() {
	p.lines = nil
	p.clearSearch() // also rebuilds (now-empty) content
	p.viewport.GotoTop()
}

// SetVisible shows or hides the pane. View returns "" while hidden.
func (p *LogPane) SetVisible(v bool) { p.visible = v }

// Visible reports whether the pane is currently shown.
func (p *LogPane) Visible() bool { return p.visible }

// Toggle flips visibility.
func (p *LogPane) Toggle() { p.visible = !p.visible }

// WordWrap reports whether lines are currently word-wrapped to the
// pane's width (off by default).
func (p LogPane) WordWrap() bool { return p.wordWrap }

// ToggleWordWrap flips word-wrap on/off and re-renders content
// accordingly.
func (p *LogPane) ToggleWordWrap() {
	p.wordWrap = !p.wordWrap
	p.refreshContent()
}

// Width reports the pane's current content width.
func (p LogPane) Width() int { return p.viewport.Width }

// Height reports the pane's current content height.
func (p LogPane) Height() int { return p.viewport.Height }

// AtTop reports whether the pane is scrolled to the very top.
func (p LogPane) AtTop() bool { return p.viewport.AtTop() }

// SearchActive reports whether the search input currently has focus --
// a host driving both a LogPane and something else off the same
// keystrokes (e.g. tui.Model's TreeView) should forward every key here
// unconditionally while this is true, the same way TreeView.SearchActive
// gates its own host.
func (p LogPane) SearchActive() bool { return p.searching }

// Init implements tea.Model.
func (p LogPane) Init() tea.Cmd { return nil }

// Update implements tea.Model. tea.WindowSizeMsg is always handled
// (bubbles/viewport.Update never resizes itself — Width/Height are plain
// fields the host must set); tea.KeyMsg is handled by handleKey (g/G,
// "/" search, n/N, else forwarded to the viewport's own default keymap
// for j/k/u/d/pgup/pgdown/left/right); anything else (e.g. mouse wheel)
// forwards straight to the viewport.
func (p LogPane) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		p.SetSize(msg.Width, msg.Height)
		return p, nil
	case tea.KeyMsg:
		return p.handleKey(msg)
	}
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

func (p LogPane) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if p.searching {
		return p.handleSearchKey(msg)
	}

	switch msg.String() {
	case "/":
		p.searching = true
		p.searchInput.Focus()
		return p, textinput.Blink
	case "w":
		p.ToggleWordWrap()
		return p, nil
	case "n":
		p.jumpToMatch(p.matchPos + 1)
		return p, nil
	case "N":
		p.jumpToMatch(p.matchPos - 1)
		return p, nil
	case "g":
		if p.pendingG {
			p.viewport.GotoTop()
			p.pendingG = false
		} else {
			p.pendingG = true
		}
		return p, nil
	case "G":
		p.viewport.GotoBottom()
		p.pendingG = false
		return p, nil
	}

	p.pendingG = false
	var cmd tea.Cmd
	p.viewport, cmd = p.viewport.Update(msg)
	return p, cmd
}

func (p LogPane) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		p.searching = false
		return p, nil
	case "esc":
		p.clearSearch()
		return p, nil
	default:
		var cmd tea.Cmd
		p.searchInput, cmd = p.searchInput.Update(msg)
		p.searchQuery = p.searchInput.Value()
		p.recomputeMatches()
		p.refreshContent() // re-highlight matches as the query changes
		if len(p.matches) > 0 {
			p.jumpToMatch(0)
		}
		return p, cmd
	}
}

// recomputeMatches finds every line containing searchQuery
// (case-insensitive substring, not fuzzy — log text is grepped, not
// fuzzy-matched the way a short resource address is).
func (p *LogPane) recomputeMatches() {
	p.matches = p.matches[:0]
	p.matchPos = 0
	if p.searchQuery == "" {
		return
	}
	q := strings.ToLower(p.searchQuery)
	for i, line := range p.lines {
		if strings.Contains(strings.ToLower(line), q) {
			p.matches = append(p.matches, i)
		}
	}
}

// jumpToMatch moves to match index pos (wrapping) and scrolls that
// line's first wrapped row into view.
func (p *LogPane) jumpToMatch(pos int) {
	if len(p.matches) == 0 {
		return
	}
	pos = ((pos % len(p.matches)) + len(p.matches)) % len(p.matches)
	p.matchPos = pos
	lineIdx := p.matches[pos]
	if lineIdx >= 0 && lineIdx < len(p.wrappedLineStarts) {
		p.viewport.SetYOffset(p.wrappedLineStarts[lineIdx])
	}
}

func (p *LogPane) clearSearch() {
	p.searching = false
	p.searchQuery = ""
	p.searchInput.SetValue("")
	p.matches = nil
	p.matchPos = 0
	p.refreshContent()
}

// refreshContent rebuilds the viewport's displayed content from lines:
// each line has every occurrence of the active search query highlighted,
// then — only when word-wrap is on — is word-wrapped to the pane's
// current width. wrappedLineStarts is rebuilt alongside so match-jumping
// lands on the right displayed row either way.
func (p *LogPane) refreshContent() {
	width := p.viewport.Width
	p.wrappedLineStarts = make([]int, len(p.lines))
	var wrapped []string
	row := 0
	for i, line := range p.lines {
		p.wrappedLineStarts[i] = row
		display := line
		if p.searchQuery != "" {
			display = highlightOccurrences(display, p.searchQuery)
		}
		if p.wordWrap {
			display = wrapLine(display, width)
		}
		sub := strings.Split(display, "\n")
		wrapped = append(wrapped, sub...)
		row += len(sub)
	}
	p.viewport.SetContent(strings.Join(wrapped, "\n"))
}

// wrapLine wraps s to width, leaving it untouched below wrapMinWidth
// (see its doc) or when there's no usable width yet. It soft-wraps at
// word boundaries first (wordwrap), then hard-wraps whatever's left
// (wrap) so a single token with no spaces — a long ARN or hash, say,
// which wordwrap alone won't break — still never exceeds width. That
// guarantee matters beyond cosmetics: bubbles/viewport enables
// horizontal scrolling for the *entire* pane the moment any one line is
// wider than its Width, so a single unbroken overlong line would
// silently defeat word-wrap for every other line too.
func wrapLine(s string, width int) string {
	if width < wrapMinWidth {
		return s
	}
	return wrap.String(wordwrap.String(s, width), width)
}

// highlightOccurrences wraps every case-insensitive occurrence of query
// in line with matchHighlightStyle. Safe to call before word-wrapping:
// wordwrap.String (like the rest of this codebase's diff rendering) is
// ANSI-aware and won't split inside the escape sequences this adds.
func highlightOccurrences(line, query string) string {
	if query == "" {
		return line
	}
	lower := strings.ToLower(line)
	q := strings.ToLower(query)
	var b strings.Builder
	i := 0
	for {
		idx := strings.Index(lower[i:], q)
		if idx < 0 {
			b.WriteString(line[i:])
			break
		}
		start := i + idx
		end := start + len(query)
		b.WriteString(line[i:start])
		b.WriteString(matchHighlightStyle.Render(line[start:end]))
		i = end
	}
	return b.String()
}

// ViewSearchBar renders the search input line (while typing) or the
// "query (n/m matches)" status line (once a query is applied), or ""
// when no search is active. style wraps the rendered text; pass nil for
// no styling.
func (p LogPane) ViewSearchBar(style func(string) string) string {
	if style == nil {
		style = func(s string) string { return s }
	}
	if p.searching {
		return style("Search output: ") + p.searchInput.View() + "\n"
	}
	if p.searchQuery != "" {
		return style(fmt.Sprintf("Search output: %q (%d/%d matches)", p.searchQuery, p.matchPos+1, len(p.matches))) + "\n"
	}
	return ""
}

// View renders the pane, or "" when hidden so a host's layout never
// reserves space for it.
func (p LogPane) View() string {
	if !p.visible {
		return ""
	}
	if p.title == "" {
		return p.viewport.View()
	}
	return p.renderTitleBar() + "\n" + p.viewport.View()
}

// titleBarLeadWidth is how many rule characters lead the title bar
// before the title text itself, e.g. "── Output ────...".
const titleBarLeadWidth = 2

// renderTitleBar renders the title bar's single line: a horizontal rule
// the full width of the pane, with the title embedded near the left
// edge. Falls back to a plain, title-less rule if the pane is too
// narrow to fit the title at all, rather than overflowing the pane's
// width.
func (p LogPane) renderTitleBar() string {
	width := p.viewport.Width
	if width <= 0 {
		return mutedTextStyle.Render(" " + p.title + " ")
	}

	label := " " + p.title + " "
	labelWidth := lipgloss.Width(label)
	trailWidth := width - titleBarLeadWidth - labelWidth
	if trailWidth < 0 {
		return mutedTextStyle.Render(strings.Repeat("─", width))
	}
	lead := strings.Repeat("─", titleBarLeadWidth)
	trail := strings.Repeat("─", trailWidth)
	return mutedTextStyle.Render(lead + label + trail)
}
