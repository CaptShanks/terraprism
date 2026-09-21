package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/CaptShanks/terraprism/internal/foldtree"
	"github.com/CaptShanks/terraprism/internal/tfplan"
)

func testModelForAdapter(width, height, diffContext int) Model {
	m := Model{diffContext: diffContext}
	m.treeView = *foldtree.NewTreeView(m)
	newTV, _ := m.treeView.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m.treeView = newTV.(foldtree.TreeView)
	return m
}

// payloadOf type-asserts a node's Payload back to rowInfo, failing the
// test loudly if it's missing -- every node the adapter builds should
// carry one.
func payloadOf(t *testing.T, n foldtree.Node) rowInfo {
	t.Helper()
	info, ok := n.Payload.(rowInfo)
	if !ok {
		t.Fatalf("node %q has no rowInfo payload (got %#v)", n.ID, n.Payload)
	}
	return info
}

// TestLeafNodeHeightMatchesRenderedText is the core height-fidelity
// check: a node's declared Height must always equal exactly what its
// cached text will render as, since foldtree's scroll/paging math is
// driven entirely by declared Height, not by re-measuring text later.
func TestLeafNodeHeightMatchesRenderedText(t *testing.T) {
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			leaf("simple", tfplan.ActionUpdate, tfplan.KindString, "old", "new"),
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)

	if len(node.Children) != 1 {
		t.Fatalf("got %d children, want 1", len(node.Children))
	}
	child := node.Children[0]
	info := payloadOf(t, child)
	if info.kind != rowLeaf {
		t.Fatalf("kind = %v, want rowLeaf", info.kind)
	}
	wantHeight := strings.Count(info.text, "\n") + 1
	if child.Height != wantHeight {
		t.Fatalf("Height = %d, want %d (derived from cached text)", child.Height, wantHeight)
	}
}

// A leaf value long enough to word-wrap against a narrow viewport must
// report a Height greater than 1, matching the actual wrapped text --
// this is the width-dependent case the design review flagged as easy to
// miss (not just multiline-string attributes wrap).
func TestWrappedLeafHeightAccountsForWrapping(t *testing.T) {
	longValue := strings.Repeat("word ", 40)
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			leaf("long", tfplan.ActionCreate, tfplan.KindString, nil, longValue),
		}, ""),
	}
	m := testModelForAdapter(40, 40, defaultDiffContext) // narrow, forces wrapping
	node := m.buildResourceNode(r)

	child := node.Children[0]
	info := payloadOf(t, child)
	wantHeight := strings.Count(info.text, "\n") + 1
	if wantHeight <= 1 {
		t.Fatalf("setup: expected the long value to actually wrap in a 40-wide viewport, got height %d", wantHeight)
	}
	if child.Height != wantHeight {
		t.Fatalf("Height = %d, want %d", child.Height, wantHeight)
	}
}

// TestContainerGetsSyntheticClosingChild verifies the container-node
// shape: Height 1 for the header (so a collapsed container is charged
// exactly one line, matching its one-line summary), plus a synthetic
// trailing child for the closing bracket that only appears when
// expanded.
func TestContainerGetsSyntheticClosingChild(t *testing.T) {
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			mapBlock("tags", tfplan.ActionUpdate,
				leaf("Name", tfplan.ActionUpdate, tfplan.KindString, "a", "b"),
			),
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)

	container := node.Children[0]
	if container.Height != 1 {
		t.Fatalf("container header Height = %d, want 1", container.Height)
	}
	if !container.Collapsible {
		t.Fatalf("container should be collapsible")
	}
	if got, want := len(container.Children), 2; got != want { // Name leaf + synthetic close
		t.Fatalf("got %d container children, want %d: %+v", got, want, container.Children)
	}
	closeChild := container.Children[len(container.Children)-1]
	closeInfo := payloadOf(t, closeChild)
	if closeInfo.kind != rowClosingBracket {
		t.Fatalf("last child should be the synthetic closing bracket, got kind %v", closeInfo.kind)
	}
	if closeChild.Height != 1 || closeChild.Collapsible {
		t.Fatalf("closing bracket node = %+v, want Height:1 Collapsible:false", closeChild)
	}

	// Collapsing the container must hide both the real child and the
	// synthetic closing bracket -- verified against a real State, since
	// that's the actual mechanism that will run in the app.
	s := foldtree.New(20)
	s.SetTree([]foldtree.Node{node})
	if got, want := len(s.Rows()), 4; got != want { // resource, container, Name, close
		t.Fatalf("expanded: got %d rows, want %d", got, want)
	}
	s.SetCollapsed(container.ID, true)
	if got, want := len(s.Rows()), 2; got != want { // resource, container (summary only)
		t.Fatalf("collapsed: got %d rows, want %d", got, want)
	}
}

// TestMultilineStringGetsBodyAndEOTChildren mirrors the container case
// for multiline strings: header Height 1, synthetic body + EOT children
// that disappear when collapsed.
func TestMultilineStringGetsBodyAndEOTChildren(t *testing.T) {
	old := strings.Repeat("line\n", 10)
	new := strings.Repeat("line\n", 9) + "changed\n"
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			leaf("script", tfplan.ActionUpdate, tfplan.KindString, strings.TrimRight(old, "\n"), strings.TrimRight(new, "\n")),
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)

	header := node.Children[0]
	info := payloadOf(t, header)
	if info.kind != rowMultilineHeader {
		t.Fatalf("kind = %v, want rowMultilineHeader", info.kind)
	}
	if header.Height != 1 {
		t.Fatalf("multiline header Height = %d, want 1", header.Height)
	}
	if got, want := len(header.Children), 2; got != want { // body + EOT
		t.Fatalf("got %d children, want %d", got, want)
	}

	bodyInfo := payloadOf(t, header.Children[0])
	if bodyInfo.kind != rowMultilineBody {
		t.Fatalf("first child should be the multiline body")
	}
	// bodyInfo.text has no trailing newline (RenderRow's contract), so
	// its line count is newlines + 1 -- the same convention leaf rows
	// use, not a bare newline count.
	wantBodyHeight := strings.Count(bodyInfo.text, "\n") + 1
	if header.Children[0].Height != wantBodyHeight {
		t.Fatalf("body Height = %d, want %d (derived from cached text)", header.Children[0].Height, wantBodyHeight)
	}

	eotInfo := payloadOf(t, header.Children[1])
	if eotInfo.kind != rowEOT || header.Children[1].Height != 1 {
		t.Fatalf("second child should be a 1-line EOT row, got %+v info=%+v", header.Children[1], eotInfo)
	}

	s := foldtree.New(20)
	s.SetTree([]foldtree.Node{node})
	expandedRows := len(s.Rows())
	s.SetCollapsed(header.ID, true)
	if got, want := len(s.Rows()), expandedRows-2; got != want {
		t.Fatalf("collapsing should hide exactly the body+EOT rows: got %d rows, want %d", got, want)
	}
}

// Regression test: renderMultilineStringBody's returned text used to
// keep its own trailing "\n" on top of the "\n" TreeView.render() always
// appends after every row, producing a spurious blank line between the
// diff body and the EOT marker, and making the body's declared Height
// one line short of what actually rendered (the exact "declared Height
// disagrees with the real render" bug class this package exists to
// prevent). Checked here against the real TreeView.View() output, not
// just re-deriving the same formula the production code uses.
func TestMultilineStringBodyHasNoBlankLineBeforeEOT(t *testing.T) {
	old := "line1\nline2\nline3"
	new := "line1\nCHANGED\nline3"
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			leaf("script", tfplan.ActionUpdate, tfplan.KindString, old, new),
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)
	m.treeView.SetTree([]foldtree.Node{node})

	var plainLines []string
	for _, line := range strings.Split(m.treeView.View(), "\n") {
		plainLines = append(plainLines, strings.TrimRight(stripANSI(line), " "))
	}

	eotIdx := -1
	lastBodyIdx := -1
	for i, line := range plainLines {
		if strings.Contains(line, "line3") {
			lastBodyIdx = i
		}
		// The closing EOT row renders as bare "EOT" (no "<<"), distinct
		// from the header's opening "script = <<EOT" marker.
		if strings.TrimSpace(line) == "EOT" {
			eotIdx = i
			break
		}
	}
	if lastBodyIdx < 0 || eotIdx < 0 {
		t.Fatalf("expected to find both the last body line and EOT in:\n%v", plainLines)
	}
	if eotIdx != lastBodyIdx+1 {
		t.Fatalf("expected EOT immediately after the last body line (no blank line between), got body at %d, EOT at %d:\n%v",
			lastBodyIdx, eotIdx, plainLines)
	}
}

// A run of consecutive unchanged (ActionNoOp) attributes must collapse
// into exactly one synthetic node, not one per hidden attribute --
// preserves the existing noise-hiding feature under the new "every
// attribute is a node" model.
func TestNoopRunBecomesOneSyntheticNode(t *testing.T) {
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			leaf("a", tfplan.ActionNoOp, tfplan.KindString, "x", "x"),
			leaf("b", tfplan.ActionNoOp, tfplan.KindString, "y", "y"),
			leaf("c", tfplan.ActionNoOp, tfplan.KindString, "z", "z"),
			leaf("changed", tfplan.ActionUpdate, tfplan.KindString, "1", "2"),
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)

	if got, want := len(node.Children), 2; got != want { // one noop-note + the changed leaf
		t.Fatalf("got %d children, want %d: %+v", got, want, node.Children)
	}
	noteInfo := payloadOf(t, node.Children[0])
	if noteInfo.kind != rowUnchangedNote {
		t.Fatalf("first child kind = %v, want rowUnchangedNote", noteInfo.kind)
	}
	if !strings.Contains(stripRenderANSI(noteInfo.text), "3 unchanged attributes hidden") {
		t.Fatalf("unexpected note text: %q", noteInfo.text)
	}
	if node.Children[0].Height != 1 {
		t.Fatalf("noop-note Height = %d, want 1", node.Children[0].Height)
	}
}

// An empty container (a map/list with zero children) isn't a
// foldable block -- isContainerAttr excludes it -- so it must fall
// through to the plain leaf path just like any scalar.
func TestEmptyContainerRendersAsLeaf(t *testing.T) {
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			{Name: "empty_map", Kind: tfplan.KindMap, Action: tfplan.ActionNoOp},
		}, ""),
	}
	// Force the "changed" path so it's not swallowed by noop-run
	// collapsing, to inspect it directly.
	r.Attributes[0].Action = tfplan.ActionCreate

	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)

	if len(node.Children) != 1 {
		t.Fatalf("got %d children, want 1", len(node.Children))
	}
	info := payloadOf(t, node.Children[0])
	if info.kind != rowLeaf {
		t.Fatalf("empty container kind = %v, want rowLeaf", info.kind)
	}
	if len(node.Children[0].Children) != 0 {
		t.Fatalf("empty container node should have no foldtree children")
	}
}

// Sensitive attributes are their own plain, single-line, non-foldable
// node.
func TestSensitiveAttrIsOwnNode(t *testing.T) {
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			{Name: "password", Kind: tfplan.KindString, Action: tfplan.ActionUpdate, Sensitive: true},
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)

	if len(node.Children) != 1 {
		t.Fatalf("got %d children, want 1", len(node.Children))
	}
	child := node.Children[0]
	info := payloadOf(t, child)
	if info.kind != rowSensitive {
		t.Fatalf("kind = %v, want rowSensitive", info.kind)
	}
	if child.Height != 1 || child.Collapsible {
		t.Fatalf("sensitive node = %+v, want Height:1 Collapsible:false", child)
	}
}

// The 'x' hotkey (m.revealSensitive) swaps a sensitive attribute's row
// text between the redacted placeholder and its real diff -- toggling
// the flag directly here (rather than driving it through a key press)
// isolates the rendering behavior from the key-handling wiring, which
// TestOutputTogglePreAndPostApply-style tests already cover for other
// toggles.
func TestSensitiveAttrRevealedShowsRealValue(t *testing.T) {
	r := tfplan.Resource{
		Address: "aws_db_instance.main",
		Attributes: withPaths([]tfplan.Attribute{
			{Name: "password", Kind: tfplan.KindString, Action: tfplan.ActionUpdate, Sensitive: true, Old: "old-secret", New: "new-secret"},
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)

	node := m.buildResourceNode(r)
	text := stripRenderANSI(payloadOf(t, node.Children[0]).text)
	if strings.Contains(text, "old-secret") || strings.Contains(text, "new-secret") {
		t.Fatalf("redacted (default) text leaked the real value: %q", text)
	}
	if !strings.Contains(text, "(sensitive value)") {
		t.Fatalf("redacted text = %q, want it to contain \"(sensitive value)\"", text)
	}

	m.revealSensitive = true
	node = m.buildResourceNode(r)
	text = stripRenderANSI(payloadOf(t, node.Children[0]).text)
	if strings.Contains(text, "(sensitive value)") {
		t.Fatalf("revealed text still redacted: %q", text)
	}
	if !strings.Contains(text, "old-secret") || !strings.Contains(text, "new-secret") {
		t.Fatalf("revealed text = %q, want it to contain both old-secret and new-secret", text)
	}
	if !strings.Contains(text, "(revealed)") {
		t.Fatalf("revealed text = %q, want a \"(revealed)\" marker distinguishing it from an ordinary attribute", text)
	}
}

// Regression guard for the buildAttributeNodes reordering: isMultilineStringAttr
// (unlike isContainerAttr and tryRenderUserdataAttr) has no Sensitive
// guard of its own, so a sensitive multi-line value (e.g. a private key)
// must never reach it -- the Sensitive check has to run first, or a
// multi-line secret would get diffed line-by-line and displayed in full
// even though revealSensitive is off.
// A sensitive multi-line value (e.g. a private key or YAML/JSON secret
// body) stays a flat, single-line "(sensitive value)" row while
// redacted, matching every other sensitive attribute -- but once
// revealed, it must go through the same foldable, per-line diff every
// ordinary multi-line value gets, rather than flattening embedded
// newlines into one unreadable %q-escaped line.
func TestSensitiveMultilineValueStaysRedactedUntilRevealed(t *testing.T) {
	oldKey := "-----BEGIN KEY-----\nold-line\n-----END KEY-----"
	newKey := "-----BEGIN KEY-----\nnew-line\n-----END KEY-----"
	r := tfplan.Resource{
		Address: "tls_private_key.main",
		Attributes: withPaths([]tfplan.Attribute{
			{Name: "private_key_pem", Kind: tfplan.KindString, Action: tfplan.ActionUpdate, Sensitive: true, Old: oldKey, New: newKey},
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)

	node := m.buildResourceNode(r)
	if len(node.Children) != 1 {
		t.Fatalf("got %d children, want 1 (multiline diffing would add EOT/body children)", len(node.Children))
	}
	info := payloadOf(t, node.Children[0])
	if info.kind != rowSensitive {
		t.Fatalf("kind = %v, want rowSensitive while redacted (must not be routed through the multiline-diff path)", info.kind)
	}
	text := stripRenderANSI(info.text)
	if strings.Contains(text, "old-line") || strings.Contains(text, "new-line") {
		t.Fatalf("redacted text leaked multiline secret content: %q", text)
	}

	m.revealSensitive = true
	node = m.buildResourceNode(r)
	info = payloadOf(t, node.Children[0])
	if info.kind != rowMultilineHeader {
		t.Fatalf("kind = %v, want rowMultilineHeader once revealed", info.kind)
	}
	if len(node.Children[0].Children) != 2 {
		t.Fatalf("got %d multiline children, want 2 (body + EOT)", len(node.Children[0].Children))
	}
	body := stripRenderANSI(payloadOf(t, node.Children[0].Children[0]).text)
	if !strings.Contains(body, "old-line") || !strings.Contains(body, "new-line") {
		t.Fatalf("revealed body = %q, want it to contain the real multiline content, diffed line by line", body)
	}

	header := stripRenderANSI(m.renderFoldHeader("", info.attr, info.keyed, false, false, 120, "<<EOT", multilineFoldSummary(info.attr)))
	if !strings.Contains(header, "(revealed)") {
		t.Fatalf("revealed header = %q, want a \"(revealed)\" marker distinguishing it from an ordinary multi-line attribute", header)
	}
}

// A minified (no literal newlines) JSON secret -- e.g. a big config
// blob stored as a JSON-encoded string -- has no line structure of its
// own to trip isMultilineStringAttr, so without pretty-printing it would
// still render as one unreadable line even once revealed. Once revealed,
// it must be pretty-printed and diffed line by line like any other
// multi-line value, not left flat.
func TestSensitiveMinifiedJSONValueRevealsAsHeredocDiff(t *testing.T) {
	oldJSON := `{"a":1,"b":"old"}`
	newJSON := `{"a":1,"b":"new"}`
	r := tfplan.Resource{
		Address: "kubectl_manifest.config",
		Attributes: withPaths([]tfplan.Attribute{
			{Name: "config_json", Kind: tfplan.KindString, Action: tfplan.ActionUpdate, Sensitive: true, Old: oldJSON, New: newJSON},
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)

	node := m.buildResourceNode(r)
	info := payloadOf(t, node.Children[0])
	if info.kind != rowSensitive {
		t.Fatalf("kind = %v, want rowSensitive while redacted", info.kind)
	}
	text := stripRenderANSI(info.text)
	if strings.Contains(text, "old") || strings.Contains(text, "new") {
		t.Fatalf("redacted text leaked JSON secret content: %q", text)
	}

	m.revealSensitive = true
	node = m.buildResourceNode(r)
	info = payloadOf(t, node.Children[0])
	if info.kind != rowMultilineHeader {
		t.Fatalf("kind = %v, want rowMultilineHeader once revealed (minified JSON must still get the heredoc diff)", info.kind)
	}
	if len(node.Children[0].Children) != 2 {
		t.Fatalf("got %d multiline children, want 2 (body + EOT)", len(node.Children[0].Children))
	}
	body := stripRenderANSI(payloadOf(t, node.Children[0].Children[0]).text)
	for _, want := range []string{`"a": 1`, `"b": "old"`, `"b": "new"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("revealed body = %q, want it to contain pretty-printed line %q", body, want)
		}
	}

	// The "(JSON)" opener label is decided in RenderRow (based on
	// isJSONContainerAttr), not inside renderFoldHeader itself -- go
	// through RenderRow directly rather than hand-picking the opener, so
	// this actually exercises that decision.
	row := foldtree.Row{Payload: rowInfo{kind: rowMultilineHeader, attr: info.attr, keyed: info.keyed}}
	header := stripRenderANSI(m.RenderRow(row, false, 120, ""))
	if !strings.Contains(header, "JSON") {
		t.Fatalf("revealed header = %q, want a JSON marker noting the diff is against a pretty-printed reformat", header)
	}
}

// A whole-subtree-sensitive container (Kind Map/List, e.g. a
// credentials block) carries its real value as a raw
// map[string]interface{}/[]interface{} on Old/New, not a string (see
// tfplan.buildAttribute) -- TryPrettyPrintJSON must marshal it directly
// without a parse step, and it must get the same heredoc treatment as a
// JSON-encoded string once revealed.
func TestSensitiveWholeSubtreeContainerRevealsAsHeredocDiff(t *testing.T) {
	oldCreds := map[string]interface{}{"user": "old-user", "pass": "old-pass"}
	newCreds := map[string]interface{}{"user": "new-user", "pass": "new-pass"}
	r := tfplan.Resource{
		Address: "aws_db_instance.main",
		Attributes: withPaths([]tfplan.Attribute{
			{Name: "credentials", Kind: tfplan.KindMap, Action: tfplan.ActionUpdate, Sensitive: true, Old: oldCreds, New: newCreds},
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	m.revealSensitive = true

	node := m.buildResourceNode(r)
	info := payloadOf(t, node.Children[0])
	if info.kind != rowMultilineHeader {
		t.Fatalf("kind = %v, want rowMultilineHeader once revealed", info.kind)
	}
	body := stripRenderANSI(payloadOf(t, node.Children[0].Children[0]).text)
	for _, want := range []string{`"user": "old-user"`, `"user": "new-user"`, `"pass": "old-pass"`, `"pass": "new-pass"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("revealed body = %q, want it to contain pretty-printed line %q", body, want)
		}
	}
}

// Synthetic IDs must be deterministic across rebuilds of the identical
// data, since foldtree's cursor-preservation-by-ID depends on it -- if
// the cursor happens to sit on a closing bracket / EOT / noop-note row
// when a rebuild happens (routine, since resize and diffContext changes
// force rebuilds), selection must survive.
func TestSyntheticIDsAreDeterministicAcrossRebuilds(t *testing.T) {
	r := tfplan.Resource{
		Address: "null_resource.a",
		Attributes: withPaths([]tfplan.Attribute{
			mapBlock("tags", tfplan.ActionUpdate, leaf("Name", tfplan.ActionUpdate, tfplan.KindString, "a", "b")),
		}, ""),
	}
	m := testModelForAdapter(120, 40, defaultDiffContext)

	node1 := m.buildResourceNode(r)
	node2 := m.buildResourceNode(r)

	close1 := node1.Children[0].Children[len(node1.Children[0].Children)-1].ID
	close2 := node2.Children[0].Children[len(node2.Children[0].Children)-1].ID
	if close1 != close2 {
		t.Fatalf("closing-bracket ID not deterministic: %q vs %q", close1, close2)
	}

	s := foldtree.New(20)
	s.SetTree([]foldtree.Node{node1})
	s.SelectIndex(len(s.Rows()) - 1) // land on the closing bracket
	selectedBefore, _ := s.SelectedID()
	if selectedBefore != close1 {
		t.Fatalf("setup: expected to land on the closing bracket, got %q", selectedBefore)
	}

	s.SetTree([]foldtree.Node{node2}) // simulate a rebuild (e.g. resize/diffContext change)
	selectedAfter, _ := s.SelectedID()
	if selectedAfter != close1 {
		t.Fatalf("selection did not survive a rebuild while sitting on a synthetic row: got %q, want %q", selectedAfter, close1)
	}
}

// Payload must round-trip through State itself, not just survive the
// adapter's own construction -- Flatten copies Node.Payload onto Row,
// and the app's render loop reads it back from there.
func TestPayloadSurvivesStateRoundTrip(t *testing.T) {
	r := tfplan.Resource{Address: "null_resource.a"}
	m := testModelForAdapter(120, 40, defaultDiffContext)
	node := m.buildResourceNode(r)

	s := foldtree.New(20)
	s.SetTree([]foldtree.Node{node})
	rows := s.Rows()
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	info, ok := rows[0].Payload.(rowInfo)
	if !ok || info.kind != rowResource || info.resource.Address != "null_resource.a" {
		t.Fatalf("Row.Payload after SetTree/Rows = %#v, want the resource's rowInfo", rows[0].Payload)
	}
}
