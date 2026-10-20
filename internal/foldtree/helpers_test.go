package foldtree

import "fmt"

// leaf builds a non-collapsible node with no children.
func leaf(id string, height int) Node {
	return Node{ID: id, Height: height}
}

// block builds a collapsible container node.
func block(id string, height int, children ...Node) Node {
	return Node{ID: id, Height: height, Collapsible: true, Children: children}
}

// flatRows builds n sibling leaf nodes, each height lines tall, with
// IDs "r0".."r{n-1}".
func flatRows(n, height int) []Node {
	nodes := make([]Node, n)
	for i := range nodes {
		nodes[i] = leaf(fmt.Sprintf("r%d", i), height)
	}
	return nodes
}

// chainOf builds a single-child chain depth nodes deep (each 1 line
// tall, collapsible except the innermost leaf), built bottom-up with a
// loop rather than recursion so it stays safe at extreme depth.
func chainOf(depth int) Node {
	if depth <= 0 {
		return Node{}
	}
	inner := Node{ID: fmt.Sprintf("c%d", depth-1), Height: 1}
	for i := depth - 2; i >= 0; i-- {
		inner = Node{ID: fmt.Sprintf("c%d", i), Height: 1, Collapsible: true, Children: []Node{inner}}
	}
	return inner
}

// countNodes recursively counts every node in a tree (including hidden
// ones), for asserting structural invariants that don't depend on
// current collapse state.
func countNodes(nodes []Node) int {
	total := len(nodes)
	for _, n := range nodes {
		total += countNodes(n.Children)
	}
	return total
}
