package foldtree

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// mouseWheelDelta matches bubbles/viewport's own default MouseWheelDelta,
// so wheel scrolling feels the same as before now that wheel events are
// converted to State.MoveMouse instead of being forwarded to
// viewport.Update.
const mouseWheelDelta = 3

// RowRenderer supplies the domain-specific parts of drawing a TreeView:
// how to draw one row, and what to show in place of the row list when
// there are none (e.g. because a search or an external filter leaves
// nothing to display).
type RowRenderer interface {
	// RenderRow returns the fully-styled string for one row (no trailing
	// newline). width is the TreeView's current content width;
	// searchQuery is the current search query ("" if none active), so a
	// renderer can highlight matched text the way a search result list
	// commonly does.
	RenderRow(row Row, selected bool, width int, searchQuery string) string
	// EmptyMessage is shown instead of the row list when there are zero
	// rows to display; searchQuery is TreeView's current search query
	// ("" if no search is active), so the message can vary accordingly.
	EmptyMessage(searchQuery string) string
}

// Searchable is implemented by a Payload value that opts its row into
// TreeView's built-in fuzzy search. Only Depth == 0 rows are ever
// matched — search narrows down which top-level items are shown, not
// their descendants; a Depth == 0 node whose Payload doesn't implement
// Searchable is simply never matched once a query is active.
type Searchable interface {
	SearchText() string
}

// TreeView is a tea.Model wrapping a State with viewport composition and
// fuzzy search over top-level rows — the generic navigation/rendering/
// search layer a host application builds its own domain-specific tree on
// top of via RowRenderer.
//
// Search is entirely self-contained: TreeView keeps the full tree handed
// to it via SetTree and internally narrows State's tree down to the
// Depth == 0 nodes matching the current query, so a host never needs to
// re-filter or rebuild anything just because the user typed into the
// search box. State() is an escape hatch for a host that needs to
// trigger tree-shape operations (ExpandSubtree, SetCollapsed, ...) from
// its own domain-specific key bindings.
//
// H/L scroll the viewport sideways -- the capital letters, since h/l
// (and the arrow keys) already collapse/expand the selected node. This
// is the only way to read a row a RowRenderer deliberately never
// word-wraps (e.g. a long update's old->new diff, or a row's own
// address line), the same escape valve LogPane offers for its own
// unwrappable content.
type TreeView struct {
	nav          State
	renderer     RowRenderer
	allRoots     []Node // the full tree last given to SetTree, before search narrowing
	extraPadding int

	viewport viewport.Model
	ready    bool
	width    int
	height   int

	searching   bool
	searchInput textinput.Model
	searchQuery string
	pendingG    bool
}

var _ tea.Model = TreeView{}

// NewTreeView returns an empty TreeView. Call SetTree once there's data
// to show, and forward a tea.WindowSizeMsg (directly or via SplitView)
// before it's displayed.
func NewTreeView(renderer RowRenderer) *TreeView {
	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.CharLimit = 100
	ti.Width = 40

	return &TreeView{
		nav:         *New(0),
		renderer:    renderer,
		searchInput: ti,
	}
}

// State returns the underlying navigation state, for host-specific key
// bindings that need direct tree-shape control (e.g. a diff-context key
// that rebuilds the tree and calls SetTree again).
func (t *TreeView) State() *State { return &t.nav }

// SetTree replaces the full displayed tree. If a search query is active,
// only its Depth == 0 nodes matching that query are actually shown (see
// Searchable) — the search narrows every subsequent SetTree call the same
// way, so a host re-filtering/re-sorting while a search is active doesn't
// need to account for the query itself.
func (t *TreeView) SetTree(roots []Node) {
	t.allRoots = roots
	t.applySearchFilter()
	t.sync()
}

// SetExtraPadding sets how many blank trailing lines TreeView both
// reports to State (for scroll-boundary math) and prints after the last
// row, keeping the two in exact sync.
func (t *TreeView) SetExtraPadding(n int) {
	if n < 0 {
		n = 0
	}
	t.extraPadding = n
	t.nav.SetExtraPadding(n)
	t.sync()
}

// SearchQuery returns the current search query ("" if none).
func (t TreeView) SearchQuery() string { return t.searchQuery }

// SearchActive reports whether the search input currently has focus.
func (t TreeView) SearchActive() bool { return t.searching }

// Width reports the current content width available to RowRenderer.
func (t TreeView) Width() int { return t.viewport.Width }

// Height reports the current content height (viewport rows).
func (t TreeView) Height() int { return t.viewport.Height }

// Init implements tea.Model.
func (t TreeView) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (t TreeView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !t.ready {
			t.viewport = viewport.New(msg.Width, msg.Height)
			t.ready = true
		} else {
			t.viewport.Width = msg.Width
			t.viewport.Height = msg.Height
		}
		t.width, t.height = msg.Width, msg.Height
		t.nav.SetHeight(t.viewport.Height)
		t.sync()
		return t, nil

	case tea.MouseMsg:
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			t.nav.MoveMouse(-mouseWheelDelta)
			t.sync()
		case tea.MouseButtonWheelDown:
			t.nav.MoveMouse(mouseWheelDelta)
			t.sync()
		case tea.MouseButtonWheelLeft:
			// A trackpad horizontal swipe or shift+wheel, reported as its
			// own button rather than a modifier on WheelUp/Down -- same
			// escape valve as the H/L keys, just via the mouse.
			t.viewport.ScrollLeft(horizontalScrollStep)
		case tea.MouseButtonWheelRight:
			t.viewport.ScrollRight(horizontalScrollStep)
		}
		return t, nil

	case tea.KeyMsg:
		return t.handleKey(msg)
	}
	return t, nil
}

func (t TreeView) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if t.searching {
		return t.handleSearchKey(msg)
	}

	switch msg.String() {
	case "up", "k":
		t.nav.MoveUp()
	case "down", "j":
		t.nav.MoveDown()
	case "enter", " ":
		if id, ok := t.nav.SelectedID(); ok {
			t.nav.ToggleCollapse(id)
		}
	case "e":
		if id, ok := t.nav.SelectedID(); ok {
			t.nav.ExpandSubtree(id)
		}
	case "E":
		t.nav.ExpandAll()
	case "c":
		t.collapseCurrent()
	case "C":
		t.nav.CollapseAll()
	case "h", "left", "backspace":
		if id, ok := t.nav.SelectedID(); ok {
			t.nav.SetCollapsed(id, true)
		}
	case "l", "right":
		if id, ok := t.nav.SelectedID(); ok {
			t.nav.SetCollapsed(id, false)
		}
	case "H":
		// Capitalized, not h/left, since those already collapse the
		// selected node -- rows this needs to reach (a long update's
		// old->new diff, a resource's own address line) are deliberately
		// never word-wrapped, so this is the only way to read the rest.
		t.viewport.ScrollLeft(horizontalScrollStep)
		return t, nil
	case "L":
		t.viewport.ScrollRight(horizontalScrollStep)
		return t, nil
	case "d", "ctrl+d":
		t.nav.MoveMouse(t.viewport.Height / 2)
	case "u", "ctrl+u":
		t.nav.MoveMouse(-(t.viewport.Height / 2))
	case "ctrl+e":
		t.nav.MoveMouse(1)
	case "ctrl+y":
		t.nav.MoveMouse(-1)
	case "pgup":
		t.nav.PageUp()
	case "pgdown":
		t.nav.PageDown()
	case "g":
		// Sticky across other keypresses until a second 'g' arrives,
		// matching the original: no other handler touches pendingG.
		if t.pendingG {
			t.nav.MoveToTop()
			t.pendingG = false
			t.sync()
		} else {
			t.pendingG = true
		}
		return t, nil
	case "G":
		t.nav.MoveToBottom()
		t.pendingG = false
	case "/":
		t.searching = true
		t.searchInput.Focus()
		t.sync()
		return t, textinput.Blink
	case "n":
		t.selectAdjacentRoot(1)
	case "N":
		t.selectAdjacentRoot(-1)
	case "esc":
		t.clearSearch()
	default:
		return t, nil
	}
	t.sync()
	return t, nil
}

func (t TreeView) handleSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		t.searching = false
		t.sync()
		return t, nil
	case "esc":
		t.searching = false
		t.searchInput.SetValue("")
		t.searchQuery = ""
		t.applySearchFilter()
		t.sync()
		return t, nil
	case "up":
		t.nav.MoveUp()
		t.sync()
		return t, nil
	case "down":
		t.nav.MoveDown()
		t.sync()
		return t, nil
	default:
		var cmd tea.Cmd
		t.searchInput, cmd = t.searchInput.Update(msg)
		t.searchQuery = t.searchInput.Value()
		t.applySearchFilter()
		t.nav.MoveToTop()
		t.sync()
		return t, cmd
	}
}

// applySearchFilter rebuilds nav's tree from allRoots, keeping only
// Depth == 0 (i.e. top-level, pre-flatten) nodes matching every
// whitespace-separated term of the current query — or every node,
// unfiltered, when the query is empty.
func (t *TreeView) applySearchFilter() {
	if t.searchQuery == "" {
		t.nav.SetTree(t.allRoots)
		return
	}
	terms := strings.Fields(strings.ToLower(t.searchQuery))
	filtered := make([]Node, 0, len(t.allRoots))
	for _, root := range t.allRoots {
		text := ""
		if s, ok := root.Payload.(Searchable); ok {
			text = strings.ToLower(s.SearchText())
		}
		allMatch := true
		for _, term := range terms {
			if !FuzzyMatch(text, term) {
				allMatch = false
				break
			}
		}
		if allMatch {
			filtered = append(filtered, root)
		}
	}
	t.nav.SetTree(filtered)
}

// selectAdjacentRoot moves the selection to the next (dir=+1) or previous
// (dir=-1) Depth==0 row, wrapping at the ends — used both for search
// match cycling (n/N) and, indirectly, whenever a caller wants to step
// between top-level items regardless of what's currently expanded.
func (t *TreeView) selectAdjacentRoot(dir int) {
	rows := t.nav.Rows()
	var roots []int
	for i, row := range rows {
		if row.Depth == 0 {
			roots = append(roots, i)
		}
	}
	if len(roots) == 0 {
		return
	}
	cur := t.nav.SelectedIndex()
	pos := 0
	for i, idx := range roots {
		if idx <= cur {
			pos = i
		} else {
			break
		}
	}
	pos = ((pos+dir)%len(roots) + len(roots)) % len(roots)
	t.nav.SelectIndex(roots[pos])
}

// collapseCurrent recursively collapses the current node's whole
// branch. If there's nothing left to collapse there (a leaf, or already
// collapsed), it climbs to the parent and collapses that branch
// instead, moving the selection up with it -- so repeated presses walk
// up the tree one level at a time. Once there's no parent left to climb
// to (already at a root), it falls back to collapsing the whole tree,
// same as pressing C directly.
func (t *TreeView) collapseCurrent() {
	idx := t.nav.SelectedIndex()
	if idx < 0 {
		return
	}
	row := t.nav.Rows()[idx]
	if row.HasChildren && !row.Collapsed {
		t.nav.CollapseSubtree(row.ID)
		return
	}

	id := row.ID
	t.nav.SelectParent()
	parentID, ok := t.nav.SelectedID()
	if !ok || parentID == id {
		t.nav.CollapseAll()
		return
	}
	t.nav.CollapseSubtree(parentID)
}

// matchStatus returns the 0-based position of the selected row's
// top-level ancestor among all currently-displayed Depth==0 rows, and how
// many there are in total — used for the "(n/m matches)" status line.
// Meaningful only while a search query is active (at that point every
// Depth==0 row is, by construction, a match).
func (t TreeView) matchStatus() (current, total int) {
	sel := t.nav.SelectedIndex()
	for i, row := range t.nav.Rows() {
		if row.Depth != 0 {
			continue
		}
		if i <= sel {
			current = total
		}
		total++
	}
	return current, total
}

func (t *TreeView) clearSearch() {
	t.searchQuery = ""
	t.searchInput.SetValue("")
	t.applySearchFilter()
}

// sync refreshes rendered content and pins the viewport's scroll offset
// to nav's, which owns cursor position and scroll offset together — the
// viewport never independently decides where to scroll (mouse wheel
// events are handled by converting them to State.MoveMouse, never by
// forwarding them into viewport.Update).
func (t *TreeView) sync() {
	if !t.ready {
		return
	}
	t.viewport.SetContent(t.render())
	t.viewport.SetYOffset(t.nav.Offset())
}

func (t *TreeView) render() string {
	rows := t.nav.Rows()
	if len(rows) == 0 {
		return t.renderer.EmptyMessage(t.searchQuery)
	}

	selected := t.nav.SelectedIndex()
	var b strings.Builder
	for i, row := range rows {
		b.WriteString(t.renderer.RenderRow(row, i == selected, t.viewport.Width, t.searchQuery))
		b.WriteString("\n")
	}
	for i := 0; i < t.extraPadding; i++ {
		b.WriteString("\n")
	}
	return b.String()
}

// ViewSearchBar renders the search input line (while typing) or the
// "query (n/m matches)" status line (once a query is applied), or ""
// when no search is active. style wraps the rendered text (e.g. with a
// host's own color); pass nil for no styling.
func (t TreeView) ViewSearchBar(style func(string) string) string {
	if style == nil {
		style = func(s string) string { return s }
	}
	if t.searching {
		return style("Search: ") + t.searchInput.View() + "\n\n"
	}
	if t.searchQuery != "" {
		current, total := t.matchStatus()
		line := "Search: \"" + t.searchQuery + "\" (" + strconv.Itoa(current+1) + "/" + strconv.Itoa(total) + " matches)"
		return style(line) + "\n\n"
	}
	return ""
}

// View implements tea.Model.
func (t TreeView) View() string {
	if !t.ready {
		return ""
	}
	return t.viewport.View()
}
