package foldtree

import (
	"fmt"
	"math/rand"
	"testing"
)

// randomTree builds a random tree shape for property testing: random
// fanout and depth, random (including zero) row heights, and nodes that
// are sometimes collapsible-with-children and sometimes deliberately
// not (to exercise the "children can never be hidden" rule). IDs are
// unique across the whole tree via a shared counter.
func randomTree(r *rand.Rand, maxDepth, maxChildren, maxNodes int) []Node {
	remaining := maxNodes
	nextID := 0

	var build func(depth int) []Node
	build = func(depth int) []Node {
		if remaining <= 0 || depth >= maxDepth {
			return nil
		}
		count := r.Intn(maxChildren + 1)
		var nodes []Node
		for i := 0; i < count && remaining > 0; i++ {
			remaining--
			id := fmt.Sprintf("n%d", nextID)
			nextID++
			children := build(depth + 1)
			nodes = append(nodes, Node{
				ID:          id,
				Height:      r.Intn(4), // 0..3, including the zero-height edge case
				Collapsible: len(children) > 0 && r.Intn(2) == 0,
				Children:    children,
			})
		}
		return nodes
	}
	return build(0)
}

func collectAllIDs(nodes []Node, into map[string]bool) {
	for _, n := range nodes {
		into[n.ID] = true
		collectAllIDs(n.Children, into)
	}
}

// TestRandomizedNavigationInvariants drives many random trees through
// many random sequences of every public operation, checking after every
// single step that the state is internally consistent. This is the
// "complex faulty structures" stress test: it doesn't know or care what
// the tree represents, only that the bookkeeping never breaks.
func TestRandomizedNavigationInvariants(t *testing.T) {
	const seeds = 30
	const stepsPerSeed = 300

	for seed := int64(0); seed < seeds; seed++ {
		seed := seed
		t.Run(fmt.Sprintf("seed=%d", seed), func(t *testing.T) {
			r := rand.New(rand.NewSource(seed))
			tree := randomTree(r, 5, 4, 200)

			allIDs := map[string]bool{}
			collectAllIDs(tree, allIDs)
			idList := make([]string, 0, len(allIDs))
			for id := range allIDs {
				idList = append(idList, id)
			}

			height := 1 + r.Intn(30)
			s := New(height)
			s.SetTree(tree)
			s.SetExtraPadding(r.Intn(20))

			for step := 0; step < stepsPerSeed; step++ {
				op := r.Intn(9)
				var movedCursor bool
				switch op {
				case 0:
					s.MoveUp()
					movedCursor = true
				case 1:
					s.MoveDown()
					movedCursor = true
				case 2:
					s.PageUp()
					movedCursor = true
				case 3:
					s.PageDown()
					movedCursor = true
				case 4:
					s.MoveToTop()
					movedCursor = true
				case 5:
					s.MoveToBottom()
					movedCursor = true
				case 6:
					s.MoveMouse(r.Intn(41) - 20) // [-20, 20]
				case 7:
					if len(idList) > 0 {
						id := idList[r.Intn(len(idList))]
						s.ToggleCollapse(id)
						movedCursor = true // rebuild may reposition the cursor
					}
				case 8:
					s.SetHeight(1 + r.Intn(30))
					movedCursor = true
				}

				checkInvariants(t, s, seed, step)
				if movedCursor {
					// Any operation that moves the cursor (directly, or
					// indirectly via a tree rebuild) must leave it
					// visible — this is the property every navigation
					// bug in this conversation violated at some boundary.
					if idx := s.SelectedIndex(); idx >= 0 && !s.RowVisible(idx) {
						t.Fatalf("seed=%d step=%d op=%d: selected row %d not visible after a cursor-moving op (offset=%d)",
							seed, step, op, idx, s.Offset())
					}
				}
			}
		})
	}
}

func checkInvariants(t *testing.T, s *State, seed int64, step int) {
	t.Helper()

	idx := s.SelectedIndex()
	n := len(s.Rows())
	if n == 0 {
		if idx != -1 {
			t.Fatalf("seed=%d step=%d: SelectedIndex = %d on an empty row set, want -1", seed, step, idx)
		}
	} else if idx < 0 || idx >= n {
		t.Fatalf("seed=%d step=%d: SelectedIndex = %d out of range [0,%d)", seed, step, idx, n)
	}

	if s.Offset() < 0 {
		t.Fatalf("seed=%d step=%d: Offset = %d, must never be negative", seed, step, s.Offset())
	}
	if s.TotalLines() < 0 {
		t.Fatalf("seed=%d step=%d: TotalLines = %d, must never be negative", seed, step, s.TotalLines())
	}
	if maxOffset := s.TotalLines() - offsetHeight(s); s.Offset() > max(0, maxOffset) {
		t.Fatalf("seed=%d step=%d: Offset = %d exceeds max scrollable offset %d (totalLines=%d)",
			seed, step, s.Offset(), max(0, maxOffset), s.TotalLines())
	}

	// Every row's line range must be non-decreasing and non-overlapping.
	prevEnd := -1
	for i := 0; i < n; i++ {
		start, end := s.RowLineRange(i)
		if start <= prevEnd {
			t.Fatalf("seed=%d step=%d: row %d starts at %d, not after previous row's end %d", seed, step, i, start, prevEnd)
		}
		if end < start-1 {
			t.Fatalf("seed=%d step=%d: row %d has inverted line range [%d,%d]", seed, step, i, start, end)
		}
		prevEnd = end
	}
}

// offsetHeight exposes the viewport height for the max-offset check
// without adding a public getter solely for this test.
func offsetHeight(s *State) int {
	top, bottom := s.VisibleRange()
	h := bottom - top + 1
	if h < 0 {
		h = 0
	}
	return h
}
