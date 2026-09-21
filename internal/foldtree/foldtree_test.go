package foldtree

import (
	"strconv"
	"testing"
)

func TestFlattenOrderAndDepth(t *testing.T) {
	tree := []Node{
		block("a", 1,
			leaf("a.1", 1),
			block("a.2", 1, leaf("a.2.1", 1)),
		),
		leaf("b", 1),
	}

	rows := Flatten(tree, func(string) bool { return false })

	wantIDs := []string{"a", "a.1", "a.2", "a.2.1", "b"}
	if len(rows) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(wantIDs), rows)
	}
	for i, want := range wantIDs {
		if rows[i].ID != want {
			t.Errorf("row %d: ID = %q, want %q", i, rows[i].ID, want)
		}
	}

	wantDepths := []int{0, 1, 1, 2, 0}
	for i, want := range wantDepths {
		if rows[i].Depth != want {
			t.Errorf("row %d (%s): Depth = %d, want %d", i, rows[i].ID, rows[i].Depth, want)
		}
	}
}

func TestFlattenCollapsedHidesChildren(t *testing.T) {
	tree := []Node{
		block("a", 1, leaf("a.1", 1), leaf("a.2", 1)),
		leaf("b", 1),
	}

	rows := Flatten(tree, func(id string) bool { return id == "a" })

	wantIDs := []string{"a", "b"}
	if len(rows) != len(wantIDs) {
		t.Fatalf("got %d rows, want %d: %+v", len(rows), len(wantIDs), rows)
	}
	for i, want := range wantIDs {
		if rows[i].ID != want {
			t.Errorf("row %d: ID = %q, want %q", i, rows[i].ID, want)
		}
	}
	if !rows[0].Collapsed {
		t.Errorf("row 'a' should report Collapsed = true")
	}
	if !rows[0].HasChildren {
		t.Errorf("row 'a' should report HasChildren = true even though collapsed")
	}
}

// A node with children but Collapsible == false must never hide them,
// regardless of what the collapse-state predicate says — Collapsible is
// the gate, not the predicate. Exercises a node whose caller forgot (or
// deliberately chose not) to mark it foldable.
func TestFlattenNonCollapsibleNodeAlwaysShowsChildren(t *testing.T) {
	tree := []Node{
		{ID: "a", Height: 1, Collapsible: false, Children: []Node{leaf("a.1", 1)}},
	}

	rows := Flatten(tree, func(string) bool { return true })

	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (non-collapsible node's child must still show): %+v", len(rows), rows)
	}
	if rows[0].Collapsed {
		t.Errorf("non-collapsible node must never report Collapsed = true")
	}
}

func TestFlattenEmptyTree(t *testing.T) {
	rows := Flatten(nil, func(string) bool { return false })
	if len(rows) != 0 {
		t.Fatalf("got %d rows for an empty tree, want 0", len(rows))
	}
}

// Payload is caller-owned data that foldtree only carries through, never
// inspects — Flatten must copy it onto the corresponding Row unchanged so
// a renderer or search predicate can type-assert it back out.
func TestFlattenCarriesPayloadThrough(t *testing.T) {
	type payload struct{ label string }

	tree := []Node{
		{ID: "a", Height: 1, Collapsible: true, Payload: payload{"root"},
			Children: []Node{{ID: "a.1", Height: 1, Payload: payload{"child"}}}},
		{ID: "b", Height: 1, Payload: nil},
	}

	rows := Flatten(tree, func(string) bool { return false })

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	if got, ok := rows[0].Payload.(payload); !ok || got.label != "root" {
		t.Errorf("row 'a': Payload = %#v, want payload{\"root\"}", rows[0].Payload)
	}
	if got, ok := rows[1].Payload.(payload); !ok || got.label != "child" {
		t.Errorf("row 'a.1': Payload = %#v, want payload{\"child\"}", rows[1].Payload)
	}
	if rows[2].Payload != nil {
		t.Errorf("row 'b': Payload = %#v, want nil", rows[2].Payload)
	}
}

// Flatten must not recurse per tree level, or a sufficiently deep chain
// would blow the goroutine stack. This is the "faulty/adversarial
// structure" this package explicitly needs to survive, since resource
// trees of unknown depth are exactly what triggered real navigation
// bugs in the past.
func TestFlattenSurvivesExtremelyDeepChain(t *testing.T) {
	const depth = 200_000
	root := chainOf(depth)

	rows := Flatten([]Node{root}, func(string) bool { return false })

	if len(rows) != depth {
		t.Fatalf("got %d rows, want %d", len(rows), depth)
	}
	if rows[0].ID != "c0" || rows[depth-1].ID != "c"+strconv.Itoa(depth-1) {
		t.Fatalf("chain endpoints wrong: first=%q last=%q", rows[0].ID, rows[depth-1].ID)
	}
}
