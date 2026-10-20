package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/CaptShanks/terraprism/internal/foldtree"
	"github.com/CaptShanks/terraprism/internal/tfplan"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripRenderANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// leaf builds a scalar attribute for tests. Path is left empty; use
// withPaths to fill in Path for a whole tree before rendering.
func leaf(name string, action tfplan.Action, kind tfplan.ValueKind, old, new any) tfplan.Attribute {
	return tfplan.Attribute{Name: name, Kind: kind, Action: action, Old: old, New: new}
}

// mapBlock builds a map-kind container attribute for tests.
func mapBlock(name string, action tfplan.Action, children ...tfplan.Attribute) tfplan.Attribute {
	return tfplan.Attribute{Name: name, Kind: tfplan.KindMap, Action: action, Children: children}
}

// withPaths recursively fills in Path (dotted, matching tfplan's real
// convert.go convention) for a hand-built attribute tree, so tests can
// construct trees by nesting literals without computing paths by hand.
func withPaths(attrs []tfplan.Attribute, parent string) []tfplan.Attribute {
	out := make([]tfplan.Attribute, len(attrs))
	for i, a := range attrs {
		path := a.Name
		if parent != "" {
			path = parent + "." + a.Name
		}
		a.Path = path
		a.Children = withPaths(a.Children, path)
		out[i] = a
	}
	return out
}

// buildTestResourceModel builds a Model with a real tree for r, via the
// same adapter + one-time default-collapse the app uses on initial load,
// so tests exercise the real code path rather than a bespoke test-only
// renderer.
func buildTestResourceModel(r tfplan.Resource, diffContext int) Model {
	m := Model{diffContext: diffContext, defaultsApplied: make(map[string]bool)}
	m.treeView = *foldtree.NewTreeView(m)
	newTV, _ := m.treeView.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.treeView = newTV.(foldtree.TreeView)

	node := m.buildResourceNode(r)
	m.treeView.SetTree([]foldtree.Node{node})
	m.applyDefaultCollapse(m.treeView.State(), []foldtree.Node{node})
	// Resources start collapsed by default in the real app; these tests
	// are about attribute-level rendering, so expand the resource root
	// itself (matching the old helper's scope, which had no resource-
	// level collapse concept at all).
	m.treeView.State().SetCollapsed(r.Address, false)
	return m
}

// renderModelRows renders every row except skipID (typically the
// resource's own header line, to match the old renderAttributeTree-only
// test scope), unselected.
func renderModelRows(m *Model, skipID string) string {
	var b strings.Builder
	width := m.treeView.Width()
	query := m.treeView.SearchQuery()
	for _, row := range m.treeView.State().Rows() {
		if row.ID == skipID {
			continue
		}
		b.WriteString(m.RenderRow(row, false, width, query))
		b.WriteString("\n")
	}
	return stripRenderANSI(b.String())
}

// renderResourceForTest renders a resource's attribute tree the way the
// app does on initial load (real adapter + one-time default collapse),
// skipping the resource's own header line.
func renderResourceForTest(r tfplan.Resource, diffContext int) string {
	m := buildTestResourceModel(r, diffContext)
	return renderModelRows(&m, r.Address)
}

// Terraform's JSON plan carries the resource's full before/after state,
// so a partially-changed resource has mostly-unchanged attributes mixed
// in with the real diff. Rendering every one of them individually — even
// muted — buried the actual change, so unchanged runs collapse into a
// single "# (N unchanged attributes hidden)" note instead, matching
// Terraform CLI's own convention.
func TestUnchangedAttributesCollapseIntoHiddenCountNote(t *testing.T) {
	r := tfplan.Resource{
		Address: "kubectl_manifest.example",
		Action:  tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{
			leaf("api_version", tfplan.ActionNoOp, tfplan.KindString, "v1", "v1"),
			leaf("force_conflicts", tfplan.ActionNoOp, tfplan.KindBool, false, false),
			leaf("live_uid", tfplan.ActionNoOp, tfplan.KindString, "abc-123", "abc-123"),
			leaf("wait_for_rollout", tfplan.ActionUpdate, tfplan.KindBool, false, true),
			leaf("timeouts", tfplan.ActionNoOp, tfplan.KindNull, nil, nil),
		}, ""),
	}

	got := renderResourceForTest(r, 0)

	if !strings.Contains(got, "# (3 unchanged attributes hidden)") {
		t.Fatalf("expected a single hidden-count note for the leading run of 3 unchanged attributes:\n%s", got)
	}
	if strings.Contains(got, "api_version") || strings.Contains(got, "force_conflicts") || strings.Contains(got, "live_uid") {
		t.Fatalf("unchanged attribute names should not be rendered individually:\n%s", got)
	}
	if !strings.Contains(got, "wait_for_rollout") {
		t.Fatalf("expected the actually-changed attribute to still render:\n%s", got)
	}
	if !strings.Contains(got, "# (1 unchanged attribute hidden)") {
		t.Fatalf("expected a singular-noun hidden-count note for the trailing run of 1:\n%s", got)
	}
}

func TestRenderGenericLargeBlockCollapsesByDefault(t *testing.T) {
	children := make([]tfplan.Attribute, 0, 35)
	for i := 0; i < 35; i++ {
		children = append(children, leaf("key"+string(rune('a'+i%26)), tfplan.ActionNoOp, tfplan.KindString, "value", "value"))
	}
	metadata := mapBlock("metadata", tfplan.ActionUpdate, children...)
	values := leaf("values", tfplan.ActionUpdate, tfplan.KindString,
		"controller:\n  replicaCount: 2\n",
		"controller:\n  replicaCount: 3\n",
	)

	r := tfplan.Resource{
		Address:    "helm_release.chart",
		Type:       "helm_release",
		Action:     tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{metadata, values}, ""),
	}

	got := renderResourceForTest(r, 0)

	if !strings.Contains(got, "▶ ~ metadata = { ... 35 attrs }") {
		t.Fatalf("expected metadata block to collapse by default:\n%s", got)
	}
	if strings.Contains(got, "keya") {
		t.Fatalf("collapsed block content should be hidden:\n%s", got)
	}
	if !strings.Contains(got, "replicaCount: 2") || !strings.Contains(got, "replicaCount: 3") {
		t.Fatalf("expected multiline string diff to show both sides:\n%s", got)
	}
}

// A collapsed container whose entire value is unknown after apply (e.g. a
// helm_release's "metadata" block, wholly unknown on update) must show
// "(known after apply)" on its own collapsed summary line -- otherwise
// that's only visible per-field once expanded, and collapsing hides the
// one piece of information ("this whole block isn't knowable yet") that
// matters most while collapsed.
func TestCollapsedComputedContainerShowsKnownAfterApply(t *testing.T) {
	children := make([]tfplan.Attribute, 0, 35)
	for i := 0; i < 35; i++ {
		children = append(children, leaf("key"+string(rune('a'+i%26)), tfplan.ActionUpdate, tfplan.KindString, "value", nil))
	}
	metadata := tfplan.Attribute{
		Name:     "metadata",
		Kind:     tfplan.KindMap,
		Action:   tfplan.ActionUpdate,
		Computed: true,
		Children: children,
	}

	r := tfplan.Resource{
		Address:    "helm_release.chart",
		Type:       "helm_release",
		Action:     tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{metadata}, ""),
	}

	got := renderResourceForTest(r, 0)

	if !strings.Contains(got, "▶ ~ metadata = { ... 35 attrs } (known after apply)") {
		t.Fatalf("expected collapsed metadata block to show (known after apply):\n%s", got)
	}
}

// A changed multi-line string is structurally a single Attribute with one
// Old and one New value — there's no "pairing" step needed the way the old
// heredoc-marker text parser needed to pair a removed block with an added
// one, so it always renders as exactly one diff block.
func TestMultilineStringRendersAsSingleDiffBlock(t *testing.T) {
	r := tfplan.Resource{
		Address: "helm_release.chart",
		Action:  tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{
			leaf("values", tfplan.ActionUpdate, tfplan.KindString,
				"controller:\n  replicaCount: 2\n",
				"controller:\n  replicaCount: 3\n"),
		}, ""),
	}

	got := renderResourceForTest(r, 0)
	if count := strings.Count(got, "<<EOT"); count != 1 {
		t.Fatalf("expected exactly one multiline diff block, got %d:\n%s", count, got)
	}
	if !strings.Contains(got, "replicaCount: 2") || !strings.Contains(got, "replicaCount: 3") {
		t.Fatalf("expected both old and new content visible:\n%s", got)
	}
}

// Large multi-line string diffs must be foldable, the same way large
// containers are: this regressed when multiline rendering was first
// introduced (always fully expanded, no collapse indicator at all).
func TestLargeMultilineStringDiffIsFoldable(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 40; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line-%d: unchanged", i))
		newLines = append(newLines, fmt.Sprintf("line-%d: unchanged", i))
	}
	newLines[20] = "line-20: changed"
	attr := leaf("values", tfplan.ActionUpdate, tfplan.KindString,
		strings.Join(oldLines, "\n"), strings.Join(newLines, "\n"))

	r := tfplan.Resource{
		Address:    "helm_release.chart",
		Action:     tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{attr}, ""),
	}

	// Collapsed by default (40 lines exceeds defaultCollapsedFoldLines).
	got := renderResourceForTest(r, 0)
	if !strings.Contains(got, "▶ ~ values = <<EOT ... 40 lines") {
		t.Fatalf("expected a collapsed fold header with a line-count summary:\n%s", got)
	}
	if strings.Contains(got, "line-0: unchanged") {
		t.Fatalf("collapsed multiline content should be hidden:\n%s", got)
	}

	// Expanding it reveals the content and flips the indicator, exactly
	// like a container fold.
	m := buildTestResourceModel(r, 0)
	valuesID := foldKey(r.Address, "values")
	if !m.treeView.State().IsCollapsed(valuesID) {
		t.Fatal("expected the multiline attribute to be a navigable, default-collapsed fold block")
	}
	m.treeView.State().SetCollapsed(valuesID, false)
	expanded := renderModelRows(&m, r.Address)

	if !strings.Contains(expanded, "▼ ~ values = <<EOT") {
		t.Fatalf("expected an expanded fold header after toggling:\n%s", expanded)
	}
	if !strings.Contains(expanded, "line-20: unchanged") || !strings.Contains(expanded, "line-20: changed") {
		t.Fatalf("expanded multiline content should show the diffed line:\n%s", expanded)
	}
}

func TestDiffContextControlsMultilineContextLines(t *testing.T) {
	oldVal := strings.Join([]string{
		"before-a: true", "before-b: true", "before-c: true",
		"target: old",
		"after-a: true", "after-b: true", "after-c: true",
	}, "\n")
	newVal := strings.Join([]string{
		"before-a: true", "before-b: true", "before-c: true",
		"target: new",
		"after-a: true", "after-b: true", "after-c: true",
	}, "\n")

	r := tfplan.Resource{
		Address:    "helm_release.chart",
		Action:     tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{leaf("values", tfplan.ActionUpdate, tfplan.KindString, oldVal, newVal)}, ""),
	}

	withoutContext := renderResourceForTest(r, 0)
	if strings.Contains(withoutContext, "before-a: true") || strings.Contains(withoutContext, "after-c: true") {
		t.Fatalf("expected zero diff context to hide far context lines:\n%s", withoutContext)
	}
	if !strings.Contains(withoutContext, "target: old") || !strings.Contains(withoutContext, "target: new") {
		t.Fatalf("expected changed lines to remain visible with zero context:\n%s", withoutContext)
	}

	withContext := renderResourceForTest(r, 3)
	for _, want := range []string{"before-a: true", "before-b: true", "before-c: true", "after-a: true", "after-b: true", "after-c: true"} {
		if !strings.Contains(withContext, want) {
			t.Fatalf("expected expanded diff context to include %q:\n%s", want, withContext)
		}
	}
}

func TestDiffContextHotkeysClampContext(t *testing.T) {
	m := NewModel(&tfplan.Plan{}, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	mm := model.(Model)

	mm, _, handled := handleKeyIncreaseDiffContext(mm)
	if !handled {
		t.Fatal("expected increase diff context key to be handled")
	}
	if got, want := mm.diffContextSize(), defaultDiffContext+diffContextStep; got != want {
		t.Fatalf("diff context after increase = %d, want %d", got, want)
	}

	for i := 0; i < 20; i++ {
		mm, _, _ = handleKeyIncreaseDiffContext(mm)
	}
	if got := mm.diffContextSize(); got != maxDiffContext {
		t.Fatalf("diff context should clamp to max %d, got %d", maxDiffContext, got)
	}

	for i := 0; i < 20; i++ {
		mm, _, _ = handleKeyDecreaseDiffContext(mm)
	}
	if got := mm.diffContextSize(); got != 0 {
		t.Fatalf("diff context should clamp to 0, got %d", got)
	}
}

// End-to-end 'x' hotkey test through the real Update() pipeline (as
// opposed to TestSensitiveAttrRevealedShowsRealValue, which calls
// buildResourceNode directly) -- mirrors TestOutputTogglePreAndPostApply's
// style for the analogous 'o' toggle.
func TestToggleSensitiveHotkeyRevealsRealValues(t *testing.T) {
	plan := &tfplan.Plan{Resources: []tfplan.Resource{
		{
			Address: "aws_db_instance.main",
			Action:  tfplan.ActionUpdate,
			Attributes: withPaths([]tfplan.Attribute{
				leafSensitive("password", tfplan.ActionUpdate, "old-secret", "new-secret"),
			}, ""),
		},
	}}
	m := NewModel(plan, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	mm := model.(Model)
	mm.treeView.State().ExpandAll() // resources start collapsed by default; the sensitive row must be visible

	view := stripRenderANSI(mm.View())
	if !strings.Contains(view, "(sensitive value)") {
		t.Fatalf("expected redacted value by default, got:\n%s", view)
	}
	if strings.Contains(view, "old-secret") || strings.Contains(view, "new-secret") {
		t.Fatalf("real value leaked before revealing:\n%s", view)
	}

	model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	mm = model.(Model)
	view = stripRenderANSI(mm.View())
	if !strings.Contains(view, "old-secret") || !strings.Contains(view, "new-secret") {
		t.Fatalf("expected 'x' to reveal the real value, got:\n%s", view)
	}

	model, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	mm = model.(Model)
	view = stripRenderANSI(mm.View())
	if !strings.Contains(view, "(sensitive value)") {
		t.Fatalf("expected a second 'x' to re-redact, got:\n%s", view)
	}
	if strings.Contains(view, "old-secret") || strings.Contains(view, "new-secret") {
		t.Fatalf("real value leaked after re-redacting:\n%s", view)
	}
}

// leafSensitive builds a sensitive scalar attribute for tests, carrying
// its real Old/New (matching tfplan.buildAttribute's actual behavior)
// rather than the redacted nil a naive test fixture might assume.
func leafSensitive(name string, action tfplan.Action, old, new any) tfplan.Attribute {
	return tfplan.Attribute{Name: name, Kind: tfplan.KindString, Action: action, Sensitive: true, Old: old, New: new}
}

// nestedMetadataResource builds a resource with a two-level-deep fold
// tree: metadata { values { nested = true } }.
func nestedMetadataResource() tfplan.Resource {
	nested := leaf("nested", tfplan.ActionUpdate, tfplan.KindBool, false, true)
	values := mapBlock("values", tfplan.ActionUpdate, nested)
	metadata := mapBlock("metadata", tfplan.ActionUpdate, values)
	return tfplan.Resource{
		Address:    "helm_release.chart",
		Action:     tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{metadata}, ""),
	}
}

func TestVisibleFoldBlocksExcludesChildrenOfCollapsedParent(t *testing.T) {
	r := nestedMetadataResource()
	m := buildTestResourceModel(r, 0)
	m.treeView.State().ExpandAll()

	metadataID := foldKey(r.Address, "metadata")
	valuesID := foldKey(r.Address, "metadata.values")

	visible := func(id string) bool {
		for _, row := range m.treeView.State().Rows() {
			if row.ID == id {
				return true
			}
		}
		return false
	}

	if !visible(valuesID) {
		t.Fatal("setup: expected metadata.values visible while parent expanded")
	}

	m.treeView.State().SetCollapsed(metadataID, true)
	if visible(valuesID) {
		t.Fatal("expected metadata.values to be hidden once its parent is collapsed")
	}
}

func TestVisibleFoldBlocksIncludesChildrenOfExpandedParent(t *testing.T) {
	r := nestedMetadataResource()
	m := buildTestResourceModel(r, 0)
	m.treeView.State().ExpandAll()

	metadataID := foldKey(r.Address, "metadata")
	valuesID := foldKey(r.Address, "metadata.values")

	mi, vi := -1, -1
	for i, row := range m.treeView.State().Rows() {
		if row.ID == metadataID {
			mi = i
		}
		if row.ID == valuesID {
			vi = i
		}
	}
	if mi < 0 || vi < 0 {
		t.Fatalf("expected both metadata and metadata.values visible (metadata=%d, values=%d)", mi, vi)
	}
	if vi <= mi {
		t.Fatalf("expected metadata.values to appear after metadata, got indices %d, %d", mi, vi)
	}
}

// ExpandSubtree/CollapseSubtree on a resource's own address must affect
// the resource and every descendant fold, regardless of nesting depth --
// the unified replacement for the old root-scope expand/collapse.
func TestScopedCollapseAffectsResourceAndDescendants(t *testing.T) {
	r := nestedMetadataResource()
	m := buildTestResourceModel(r, 0)
	m.treeView.State().ExpandAll()

	metadataID := foldKey(r.Address, "metadata")

	m.treeView.State().CollapseSubtree(r.Address)
	if !m.treeView.State().IsCollapsed(r.Address) || !m.treeView.State().IsCollapsed(metadataID) {
		t.Fatalf("expected resource-scope collapse to fold the resource and its descendants")
	}

	m.treeView.State().ExpandSubtree(r.Address)
	if m.treeView.State().IsCollapsed(r.Address) || m.treeView.State().IsCollapsed(metadataID) {
		t.Fatalf("expected resource-scope expand to unfold the resource and its descendants")
	}
}

// Collapsing a specific sub-block (not the resource root) must not leak
// to a sibling fold block.
func TestScopedCollapseOnSubBlockDoesNotAffectSibling(t *testing.T) {
	nested := leaf("nested", tfplan.ActionUpdate, tfplan.KindBool, false, true)
	values := mapBlock("values", tfplan.ActionUpdate, nested)
	metadata := mapBlock("metadata", tfplan.ActionUpdate, values)
	set := mapBlock("set", tfplan.ActionUpdate, leaf("value", tfplan.ActionUpdate, tfplan.KindBool, false, true))
	r := tfplan.Resource{
		Address:    "helm_release.chart",
		Action:     tfplan.ActionUpdate,
		Attributes: withPaths([]tfplan.Attribute{metadata, set}, ""),
	}
	m := buildTestResourceModel(r, 0)
	m.treeView.State().ExpandAll()

	metadataID := foldKey(r.Address, "metadata")
	valuesID := foldKey(r.Address, "metadata.values")
	setID := foldKey(r.Address, "set")

	m.treeView.State().CollapseSubtree(metadataID)

	if !m.treeView.State().IsCollapsed(metadataID) || !m.treeView.State().IsCollapsed(valuesID) {
		t.Fatalf("expected the selected fold and its descendant to collapse")
	}
	if m.treeView.State().IsCollapsed(setID) {
		t.Fatalf("did not expect sibling fold 'set' to collapse")
	}
}

// Shift+E/Shift+C (global expand/collapse) must reach nested folds, not
// just resource roots.
func TestExpandAndCollapseEverythingAffectsNestedFoldsToo(t *testing.T) {
	metadata := mapBlock("metadata", tfplan.ActionUpdate,
		mapBlock("values", tfplan.ActionUpdate, leaf("nested", tfplan.ActionUpdate, tfplan.KindBool, false, true)))
	spec := mapBlock("spec", tfplan.ActionUpdate, leaf("replicas", tfplan.ActionUpdate, tfplan.KindNumber, "2", "3"))

	resources := []tfplan.Resource{
		{Address: "helm_release.chart", Action: tfplan.ActionUpdate, Attributes: withPaths([]tfplan.Attribute{metadata}, "")},
		{Address: "kubectl_manifest.vmagent", Action: tfplan.ActionUpdate, Attributes: withPaths([]tfplan.Attribute{spec}, "")},
	}
	m := newTestModel(resources)

	ids := []string{
		"helm_release.chart",
		foldKey("helm_release.chart", "metadata"),
		foldKey("helm_release.chart", "metadata.values"),
		"kubectl_manifest.vmagent",
		foldKey("kubectl_manifest.vmagent", "spec"),
	}

	updated := pressKey(m, "E")
	for _, id := range ids {
		if updated.treeView.State().IsCollapsed(id) {
			t.Fatalf("expected %q to be expanded by global expand", id)
		}
	}

	collapsed := pressKey(updated, "C")
	for _, id := range ids {
		if !collapsed.treeView.State().IsCollapsed(id) {
			t.Fatalf("expected %q to be collapsed by global collapse", id)
		}
	}
}

func TestViewHelpFooterUsesCompactTextForNarrowWidths(t *testing.T) {
	m := Model{width: 72}
	got := m.viewHelpFooter()
	if lipgloss.Width(got) > 68 {
		t.Fatalf("help footer width = %d, want <= 68: %q", lipgloss.Width(got), got)
	}
	if strings.Contains(got, "expand/collapse scope") {
		t.Fatalf("expected compact help footer, got %q", got)
	}
}
