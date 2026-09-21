package foldtree

// State holds a flattened, collapsible tree's cursor position and
// scroll offset together, so keyboard-driven cursor movement and
// mouse-wheel scrolling can never drift into two disagreeing views of
// "where we are." Every mutating method reconciles both before
// returning.
type State struct {
	roots     []Node
	parent    map[string]string
	collapsed map[string]bool

	rows       []Row
	lineStarts []int
	rowIndex   map[string]int

	extraPadding int
	totalLines   int

	cursor int // index into rows; -1 if rows is empty
	offset int // viewport's top line
	height int // viewport height in lines
}

// New creates a State for a viewport of the given height, in lines.
func New(height int) *State {
	if height < 0 {
		height = 0
	}
	return &State{
		collapsed: make(map[string]bool),
		cursor:    -1,
		height:    height,
	}
}

// SetHeight changes the viewport height, in lines.
func (s *State) SetHeight(h int) {
	if h < 0 {
		h = 0
	}
	s.height = h
	s.ensureVisible()
}

// SetTree replaces the tree, preserving the current selection by ID
// when possible. If the previously selected row is no longer visible
// (an ancestor was collapsed, or the row was removed entirely),
// selection walks up toward the nearest still-visible ancestor; failing
// that, it clamps to a valid row index.
func (s *State) SetTree(roots []Node) {
	s.roots = roots
	s.parent = buildParentMap(roots)
	s.refreshPreservingSelection()
}

// SetExtraPadding sets how many additional lines of scrollable content
// exist below the last row (e.g. trailing chrome/footer text that isn't
// itself a navigable row). Total scrollable extent is the sum of all
// row heights plus this padding.
func (s *State) SetExtraPadding(n int) {
	if n < 0 {
		n = 0
	}
	s.extraPadding = n
	s.recomputeTotalLines()
	s.clampOffset()
}

// IsCollapsed reports whether the node with the given ID has been
// marked collapsed via SetCollapsed/ToggleCollapse/CollapseAll. State
// doesn't track which IDs belong to collapsible nodes, so this can
// report true for a non-collapsible node's ID if a caller collapses it
// directly -- that's harmless in practice, since Flatten independently
// checks the node's own Collapsible field before ever consulting this,
// so a non-collapsible node's children are never actually hidden
// regardless of what this method reports for it.
func (s *State) IsCollapsed(id string) bool {
	return s.collapsed[id]
}

// SetCollapsed sets the collapsed state of the node with the given ID.
func (s *State) SetCollapsed(id string, collapsed bool) {
	if collapsed {
		s.collapsed[id] = true
	} else {
		delete(s.collapsed, id)
	}
	s.refreshPreservingSelection()
}

// ToggleCollapse flips the collapsed state of the node with the given ID.
func (s *State) ToggleCollapse(id string) {
	s.SetCollapsed(id, !s.collapsed[id])
}

// CollapseAll collapses every collapsible node in the tree, including
// ones not currently visible.
func (s *State) CollapseAll() {
	for id := range collapsibleIDs(s.roots) {
		s.collapsed[id] = true
	}
	s.refreshPreservingSelection()
}

// ExpandAll expands every node in the tree.
func (s *State) ExpandAll() {
	s.collapsed = make(map[string]bool)
	s.refreshPreservingSelection()
}

// ExpandSubtree recursively expands the node with the given ID and
// every one of its descendants, without affecting any other branch of
// the tree. A no-op if no node has that ID.
func (s *State) ExpandSubtree(id string) {
	n := findNode(s.roots, id)
	if n == nil {
		return
	}
	setSubtreeCollapsed(s.collapsed, n, false)
	s.refreshPreservingSelection()
}

// CollapseSubtree recursively collapses the node with the given ID and
// every one of its descendants, without affecting any other branch of
// the tree. A no-op if no node has that ID.
func (s *State) CollapseSubtree(id string) {
	n := findNode(s.roots, id)
	if n == nil {
		return
	}
	setSubtreeCollapsed(s.collapsed, n, true)
	s.refreshPreservingSelection()
}

// SetFoldLevel sets every collapsible node's collapsed state purely
// from its depth: nodes shallower than level stay expanded, nodes at or
// deeper than level are collapsed. SetFoldLevel(0) collapses every
// top-level collapsible node (equivalent to CollapseAll for shallow
// enough trees); a level at or beyond the tree's deepest node expands
// everything (equivalent to ExpandAll).
//
// This is the "step the whole tree's fold depth in or out by one level"
// operation familiar from outline/code-folding UIs (e.g. vim's
// zr/zm/zR/zM) — it always recomputes collapse state from scratch, so
// it's unaffected by cursor position or any previous ad hoc
// SetCollapsed/ToggleCollapse calls, and it applies tree-wide regardless
// of where the current selection happens to be.
func (s *State) SetFoldLevel(level int) {
	if level < 0 {
		level = 0
	}

	type frame struct {
		node  *Node
		depth int
	}
	collapsed := make(map[string]bool)
	stack := make([]frame, 0, len(s.roots))
	for i := range s.roots {
		stack = append(stack, frame{&s.roots[i], 0})
	}
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.node.Collapsible && f.depth >= level {
			collapsed[f.node.ID] = true
		}
		for i := range f.node.Children {
			stack = append(stack, frame{&f.node.Children[i], f.depth + 1})
		}
	}

	s.collapsed = collapsed
	s.refreshPreservingSelection()
}

// refreshPreservingSelection re-flattens the tree under the current
// collapse state and tries to keep the same row selected, then
// reconciles the viewport to make sure the selection is visible.
func (s *State) refreshPreservingSelection() {
	prevID, hadSelection := s.SelectedID()
	s.rebuildRows()
	switch {
	case hadSelection:
		s.restoreSelection(prevID)
	case len(s.rows) > 0:
		s.cursor = 0
	default:
		s.cursor = -1
	}
	s.ensureVisible()
}

func (s *State) rebuildRows() {
	s.rows = Flatten(s.roots, func(id string) bool { return s.collapsed[id] })
	s.lineStarts = make([]int, len(s.rows))
	s.rowIndex = make(map[string]int, len(s.rows))

	line := 0
	for i, r := range s.rows {
		s.lineStarts[i] = line
		s.rowIndex[r.ID] = i
		line += effectiveHeight(r.Height)
	}
	s.recomputeTotalLines()
}

func (s *State) recomputeTotalLines() {
	sum := 0
	if n := len(s.rows); n > 0 {
		sum = s.lineStarts[n-1] + effectiveHeight(s.rows[n-1].Height)
	}
	s.totalLines = sum + s.extraPadding
}

// restoreSelection tries to keep prevID selected after a rebuild; if
// it's gone, it walks the old tree's parent chain looking for the
// nearest ancestor that still has a visible row.
func (s *State) restoreSelection(prevID string) {
	if idx, ok := s.rowIndex[prevID]; ok {
		s.cursor = idx
		return
	}
	for id, hasParent := prevID, true; hasParent; {
		var next string
		next, hasParent = s.parent[id]
		if !hasParent {
			break
		}
		if idx, ok := s.rowIndex[next]; ok {
			s.cursor = idx
			return
		}
		id = next
	}
	s.clampCursor()
}

func (s *State) clampCursor() {
	switch {
	case len(s.rows) == 0:
		s.cursor = -1
	case s.cursor < 0:
		s.cursor = 0
	case s.cursor >= len(s.rows):
		s.cursor = len(s.rows) - 1
	}
}
