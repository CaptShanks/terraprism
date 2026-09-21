package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/CaptShanks/terraprism/internal/tfplan"
)

func threeResourcesWithFolds() []tfplan.Resource {
	one := leaf("x", tfplan.ActionUpdate, tfplan.KindNumber, "1", "1")
	two := mapBlock("metadata", tfplan.ActionUpdate,
		mapBlock("values", tfplan.ActionUpdate, leaf("nested", tfplan.ActionUpdate, tfplan.KindBool, false, true)))
	three := leaf("x", tfplan.ActionUpdate, tfplan.KindNumber, "3", "3")

	return []tfplan.Resource{
		{Address: "a.one", Action: tfplan.ActionUpdate, Attributes: withPaths([]tfplan.Attribute{one}, "")},
		{Address: "b.two", Action: tfplan.ActionUpdate, Attributes: withPaths([]tfplan.Attribute{two}, "")},
		{Address: "c.three", Action: tfplan.ActionUpdate, Attributes: withPaths([]tfplan.Attribute{three}, "")},
	}
}

// newTestModel builds a ready Model (real WindowSizeMsg flow, so the
// tree view's tree is actually built) over the given resources.
func newTestModel(resources []tfplan.Resource) Model {
	m := NewModel(&tfplan.Plan{Resources: resources}, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return model.(Model)
}

// selectRowByID moves the selection directly to the row with the given
// ID, wherever it currently sits in the tree view's flattened list --
// robust regardless of what else is expanded/collapsed, unlike a
// hardcoded row index (which shifts as soon as any resource's attributes
// appear).
func selectRowByID(m *Model, id string) {
	nav := m.treeView.State()
	for i, row := range nav.Rows() {
		if row.ID == id {
			nav.SelectIndex(i)
			return
		}
	}
}

// pressKey drives one keypress through the full Update() pipeline,
// exactly as a real terminal keystroke would.
func pressKey(m Model, key string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated.(Model)
}

// `e` scopes to the highlighted item: expand cursor's resource and its
// sub-folds, but leave siblings untouched.
func TestExpandAllScopesToHighlightedItem(t *testing.T) {
	m := newTestModel(threeResourcesWithFolds())
	selectRowByID(&m, "b.two")

	updated := pressKey(m, "e")
	nav := updated.treeView.State()

	if nav.IsCollapsed("b.two") {
		t.Fatalf("cursor's resource (b.two) should be expanded")
	}
	if !nav.IsCollapsed("a.one") || !nav.IsCollapsed("c.three") {
		t.Fatalf("siblings should NOT have been expanded by scoped `e`")
	}
	if nav.IsCollapsed(foldKey("b.two", "metadata")) {
		t.Fatalf("sub-fold metadata should be expanded after `e`")
	}
	if nav.IsCollapsed(foldKey("b.two", "metadata.values")) {
		t.Fatalf("sub-fold metadata.values should be expanded after `e`")
	}
}

// `c` collapses just the cursor's item and its sub-folds.
func TestCollapseAllScopesToHighlightedItem(t *testing.T) {
	m := newTestModel(threeResourcesWithFolds())
	m.treeView.State().ExpandAll()
	selectRowByID(&m, "b.two")

	updated := pressKey(m, "c")
	nav := updated.treeView.State()

	if !nav.IsCollapsed("b.two") {
		t.Fatalf("cursor's resource should be collapsed")
	}
	if nav.IsCollapsed("a.one") || nav.IsCollapsed("c.three") {
		t.Fatalf("siblings should remain expanded")
	}
}

// Shift+E is the global expand-all.
func TestExpandEverythingIsGlobal(t *testing.T) {
	resources := threeResourcesWithFolds()
	m := newTestModel(resources)
	selectRowByID(&m, "b.two")

	updated := pressKey(m, "E")
	nav := updated.treeView.State()

	for _, r := range resources {
		if nav.IsCollapsed(r.Address) {
			t.Fatalf("resource %s should be expanded by `E`", r.Address)
		}
	}
}

// Shift+C is the global collapse-all.
func TestCollapseEverythingIsGlobal(t *testing.T) {
	resources := threeResourcesWithFolds()
	m := newTestModel(resources)
	m.treeView.State().ExpandAll()
	selectRowByID(&m, "b.two")

	updated := pressKey(m, "C")
	nav := updated.treeView.State()

	for _, r := range resources {
		if !nav.IsCollapsed(r.Address) {
			t.Fatalf("resource %s should be collapsed by `C`", r.Address)
		}
	}
}

// Inside a sub-fold, `e` still scopes to that fold/resource, not siblings.
func TestExpandAllInsideSubBlockKeepsScope(t *testing.T) {
	m := newTestModel(threeResourcesWithFolds())
	m.treeView.State().SetCollapsed("b.two", false) // expand the resource itself first
	selectRowByID(&m, foldKey("b.two", "metadata"))

	updated := pressKey(m, "e")

	if !updated.treeView.State().IsCollapsed("a.one") {
		t.Fatalf("sibling a.one should NOT expand from sub-block-scoped `e`")
	}
}
