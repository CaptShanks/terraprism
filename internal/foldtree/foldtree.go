// Package foldtree implements cursor and scroll-offset navigation over a
// generic, multi-level collapsible tree, rendered as a linear sequence
// of variable-height rows (a "fold tree": any node can hide its children
// behind a collapsed/expanded toggle).
//
// It knows nothing about what a node represents — no text, no styling,
// no application data — only tree shape (via Node), collapse state, and
// each row's height in terminal lines. That isolation is deliberate:
// cursor position and scroll offset live together in one State value,
// reconciled by every mutation, so keyboard-driven selection and
// mouse-wheel scrolling can never drift into two disagreeing answers to
// "where are we" — which is the source of every navigation bug this
// package exists to eliminate.
package foldtree

// Node is one item in a generic, multi-level collapsible tree. Callers
// adapt their own data into Nodes; foldtree has no opinion about what
// content a node represents.
//
// ID should be unique within a tree and stable across rebuilds (e.g.
// derived from a stable path), so State can preserve the current
// selection when the tree is replaced or re-collapsed. Height is the
// number of terminal lines this node's own row occupies when rendered
// (independent of its children, which only appear as separate rows when
// the node is expanded). A negative Height is treated as zero.
//
// Payload is caller-owned data associated with this node — foldtree never
// inspects it, only carries it through to the corresponding Row so a
// renderer or search predicate can type-assert it back out.
type Node struct {
	ID          string
	Height      int
	Collapsible bool
	Children    []Node
	Payload     any
}

// Row is one visible line-item after flattening a tree through a
// collapse-state predicate: a node whose ancestors are all expanded.
type Row struct {
	ID          string
	Depth       int
	Height      int
	HasChildren bool
	Collapsed   bool
	Payload     any
}

// Flatten walks roots in document (pre-)order, skipping the children of
// any node for which isCollapsed reports true, and returns one Row per
// visible node. A node whose Collapsible field is false is never
// collapsed, regardless of isCollapsed, so its children always show.
//
// Flatten walks with an explicit stack rather than recursion, so it
// stays safe for pathologically deep trees.
func Flatten(roots []Node, isCollapsed func(id string) bool) []Row {
	type frame struct {
		node  *Node
		depth int
	}

	rows := make([]Row, 0, len(roots))
	stack := make([]frame, 0, len(roots))
	for i := len(roots) - 1; i >= 0; i-- {
		stack = append(stack, frame{&roots[i], 0})
	}

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := f.node

		collapsed := n.Collapsible && isCollapsed != nil && isCollapsed(n.ID)
		rows = append(rows, Row{
			ID:          n.ID,
			Depth:       f.depth,
			Height:      n.Height,
			HasChildren: len(n.Children) > 0,
			Collapsed:   collapsed,
			Payload:     n.Payload,
		})

		if !collapsed {
			for i := len(n.Children) - 1; i >= 0; i-- {
				stack = append(stack, frame{&n.Children[i], f.depth + 1})
			}
		}
	}
	return rows
}

// buildParentMap returns, for every non-root node in the tree, the ID of
// its immediate parent. Roots have no entry. Used to walk upward toward
// the nearest still-visible ancestor when the currently selected row
// disappears (e.g. an ancestor was collapsed, or the tree was replaced).
func buildParentMap(roots []Node) map[string]string {
	type frame struct {
		node     *Node
		parentID string
	}

	m := make(map[string]string)
	stack := make([]frame, 0, len(roots))
	for i := range roots {
		stack = append(stack, frame{&roots[i], ""})
	}

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if f.parentID != "" {
			m[f.node.ID] = f.parentID
		}
		for i := range f.node.Children {
			stack = append(stack, frame{&f.node.Children[i], f.node.ID})
		}
	}
	return m
}

// collapsibleIDs returns the IDs of every collapsible node in the tree,
// including ones currently hidden behind a collapsed ancestor. Used by
// CollapseAll, which must be able to collapse nodes it can't currently
// see a row for.
func collapsibleIDs(roots []Node) map[string]struct{} {
	ids := make(map[string]struct{})
	stack := make([]*Node, 0, len(roots))
	for i := range roots {
		stack = append(stack, &roots[i])
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.Collapsible {
			ids[n.ID] = struct{}{}
		}
		for i := range n.Children {
			stack = append(stack, &n.Children[i])
		}
	}
	return ids
}

// findNode returns a pointer to the first node with the given ID
// anywhere in the tree (searched structurally, so it finds nodes hidden
// behind a collapsed ancestor too), or nil if none match.
func findNode(roots []Node, id string) *Node {
	stack := make([]*Node, 0, len(roots))
	for i := range roots {
		stack = append(stack, &roots[i])
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.ID == id {
			return n
		}
		for i := range n.Children {
			stack = append(stack, &n.Children[i])
		}
	}
	return nil
}

// setSubtreeCollapsed sets collapsed to val for every collapsible node
// in root's subtree (including root itself), leaving entries for nodes
// outside that subtree untouched.
func setSubtreeCollapsed(collapsed map[string]bool, root *Node, val bool) {
	stack := []*Node{root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n.Collapsible {
			if val {
				collapsed[n.ID] = true
			} else {
				delete(collapsed, n.ID)
			}
		}
		for i := range n.Children {
			stack = append(stack, &n.Children[i])
		}
	}
}

func effectiveHeight(h int) int {
	if h < 0 {
		return 0
	}
	return h
}
