package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/CaptShanks/terraprism/internal/parser"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripRenderANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func renderExpandedForTest(r parser.Resource, lines []string) string {
	return renderExpandedWithDiffContextForTest(r, lines, 0)
}

func renderExpandedWithDiffContextForTest(r parser.Resource, lines []string, diffContext int) string {
	r.RawLines = append([]string{`  ~ resource "test" "example" {`}, lines...)
	m := Model{
		viewport:     viewport.New(120, 40),
		foldedBlocks: make(map[string]bool),
		blockCursor:  -1,
		diffContext:  diffContext,
	}
	var b strings.Builder
	lineCount := 0
	m.renderExpandedContent(&b, r, false, &lineCount)
	return stripRenderANSI(b.String())
}

func renderedHasLine(rendered, line string) bool {
	for _, renderedLine := range strings.Split(rendered, "\n") {
		if renderedLine == line {
			return true
		}
	}
	return false
}

func TestRenderHeredocPreservesYAMLListIndentation(t *testing.T) {
	r := parser.Resource{Type: "kubectl_manifest", Action: parser.ActionUpdate}
	lines := []string{
		`      ~ yaml_body_parsed = <<-EOT`,
		`            spec:`,
		`              affinity:`,
		`                nodeAffinity:`,
		`                  requiredDuringSchedulingIgnoredDuringExecution:`,
		`                    nodeSelectorTerms:`,
		`                    - matchExpressions:`,
		`                      - key: dedicated`,
		`                        operator: In`,
		`                        values:`,
		`                        - utility`,
		`        EOT`,
	}

	got := renderExpandedForTest(r, lines)

	for _, want := range []string{
		`                    - matchExpressions:`,
		`                      - key: dedicated`,
		`                        - utility`,
	} {
		if !renderedHasLine(got, want) {
			t.Fatalf("rendered heredoc missing correctly indented line %q:\n%s", want, got)
		}
	}
	if renderedHasLine(got, `                  - matchExpressions:`) {
		t.Fatalf("rendered heredoc shifted YAML list indentation left:\n%s", got)
	}
}

func TestRenderHeredocPreservesTerraformDiffPrefixColumn(t *testing.T) {
	r := parser.Resource{Type: "kubectl_manifest", Action: parser.ActionUpdate}
	lines := []string{
		`      ~ yaml_body_parsed = <<-EOT`,
		`            spec:`,
		`              remoteWrite:`,
		`              - url: http://internal-write-endpoint.example.local/api/v1/write`,
		`              - url: http://external-write-endpoint.example.com/api/v1/write`,
		`          +   - url: http://external-write-endpoint.example.com/api/v1/write`,
		`        EOT`,
	}

	got := renderExpandedForTest(r, lines)
	want := `          +   - url: http://external-write-endpoint.example.com/api/v1/write`
	if !strings.Contains(got, want) {
		t.Fatalf("rendered heredoc missing Terraform diff-prefixed YAML line %q:\n%s", want, got)
	}
}

func TestRenderGenericLargeBlockCollapsesByDefault(t *testing.T) {
	r := parser.Resource{Type: "helm_release", Address: "helm_release.chart", Action: parser.ActionUpdate}
	lines := []string{
		`      ~ metadata                   = {`,
		`          ~ app_version    = "v1.132.0" -> (known after apply)`,
		`          ~ chart          = "example-chart" -> (known after apply)`,
		`          ~ first_deployed = 1770864889 -> (known after apply)`,
		`          ~ notes          = <<-EOT`,
		`                1. Get the application URL by running these commands:`,
		`                  export POD_NAME=$(kubectl get pods --namespace victoria-metrics)`,
		`            EOT -> (known after apply)`,
		`          ~ values         = jsonencode(`,
		`                {`,
	}
	for i := 0; i < 35; i++ {
		lines = append(lines, `                  key = "value"`)
	}
	lines = append(lines,
		`                }`,
		`            ) -> (known after apply)`,
		`        }`,
		`      ~ values = [`,
		`          - <<-EOT`,
		`              controller:`,
		`                replicaCount: 2`,
		`            EOT,`,
		`          + <<-EOT`,
		`              controller:`,
		`                replicaCount: 3`,
		`            EOT,`,
		`        ]`,
	)

	r.RawLines = append([]string{`  ~ resource "test" "example" {`}, lines...)
	blocks := findFoldBlocks(r, r.RawLines[1:])
	foundValuesFold := false
	for _, block := range blocks {
		if strings.Contains(r.RawLines[block.Start+1], `values = [`) {
			foundValuesFold = true
			break
		}
	}
	if !foundValuesFold {
		t.Fatalf("expected top-level values list to be foldable, got %#v", blocks)
	}

	got := renderExpandedForTest(r, lines)

	for _, want := range []string{
		`      ▶ ~ metadata                   = { ... (47 lines)`,
		`      ▼ ~ values = [`,
		`          ▼ ~ heredoc diff <<-EOT (2 → 2 lines)`,
		`replicaCount: 2`,
		`replicaCount: 3`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered helm release missing %q:\n%s", want, got)
		}
	}
	for _, hidden := range []string{`app_version`, `Get the application URL`, `key = "value"`} {
		if strings.Contains(got, hidden) {
			t.Fatalf("rendered plan still contains collapsed block content %q:\n%s", hidden, got)
		}
	}
	for _, separate := range []string{`          ▶ - <<-EOT`, `          ▶ + <<-EOT`, `          ▼ - <<-EOT`, `          ▼ + <<-EOT`} {
		if strings.Contains(got, separate) {
			t.Fatalf("rendered plan still contains separate heredoc fold %q:\n%s", separate, got)
		}
	}
}

func TestPairedHeredocsUseSingleDiffFold(t *testing.T) {
	r := parser.Resource{
		Address: "helm_release.chart",
		Action:  parser.ActionUpdate,
		RawLines: []string{
			`  ~ resource "helm_release" "chart" {`,
			`      ~ values = [`,
			`          - <<-EOT`,
			`              controller:`,
			`                replicaCount: 2`,
			`            EOT,`,
			`          + <<-EOT`,
			`              controller:`,
			`                replicaCount: 3`,
			`            EOT,`,
			`        ]`,
		},
	}

	blocks := findFoldBlocks(r, r.RawLines[1:])
	if len(blocks) != 2 {
		t.Fatalf("expected values fold and heredoc diff fold, got %#v", blocks)
	}
	if !blocks[1].HeredocPair {
		t.Fatalf("expected second fold to be paired heredoc diff, got %#v", blocks[1])
	}

	got := renderExpandedForTest(r, r.RawLines[1:])
	if !strings.Contains(got, `▼ ~ heredoc diff <<-EOT (2 → 2 lines)`) {
		t.Fatalf("rendered plan missing combined heredoc diff fold:\n%s", got)
	}
	if strings.Contains(got, `▼ - <<-EOT`) || strings.Contains(got, `▼ + <<-EOT`) {
		t.Fatalf("rendered plan contains separate heredoc folds:\n%s", got)
	}
}

func TestDiffContextControlsHeredocContextLines(t *testing.T) {
	r := parser.Resource{
		Address: "helm_release.chart",
		Action:  parser.ActionUpdate,
	}
	lines := []string{
		`      ~ values = [`,
		`          - <<-EOT`,
		`              before-a: true`,
		`              before-b: true`,
		`              before-c: true`,
		`              target: old`,
		`              after-a: true`,
		`              after-b: true`,
		`              after-c: true`,
		`            EOT,`,
		`          + <<-EOT`,
		`              before-a: true`,
		`              before-b: true`,
		`              before-c: true`,
		`              target: new`,
		`              after-a: true`,
		`              after-b: true`,
		`              after-c: true`,
		`            EOT,`,
		`        ]`,
	}

	withoutContext := renderExpandedWithDiffContextForTest(r, lines, 0)
	if strings.Contains(withoutContext, `before-a: true`) || strings.Contains(withoutContext, `after-c: true`) {
		t.Fatalf("expected zero diff context to hide far context lines:\n%s", withoutContext)
	}
	if !strings.Contains(withoutContext, `target: old`) || !strings.Contains(withoutContext, `target: new`) {
		t.Fatalf("expected changed lines to remain visible with zero context:\n%s", withoutContext)
	}

	withContext := renderExpandedWithDiffContextForTest(r, lines, 3)
	for _, want := range []string{`before-a: true`, `before-b: true`, `before-c: true`, `after-a: true`, `after-b: true`, `after-c: true`} {
		if !strings.Contains(withContext, want) {
			t.Fatalf("expected expanded diff context to include %q:\n%s", want, withContext)
		}
	}
}

func TestDiffContextHotkeysClampContext(t *testing.T) {
	m := Model{
		plan:        &parser.Plan{},
		viewport:    viewport.New(80, 20),
		diffContext: defaultDiffContext,
	}

	m, _, handled := handleKeyIncreaseDiffContext(m)
	if !handled {
		t.Fatal("expected increase diff context key to be handled")
	}
	if got, want := m.diffContextSize(), defaultDiffContext+diffContextStep; got != want {
		t.Fatalf("diff context after increase = %d, want %d", got, want)
	}

	for i := 0; i < 20; i++ {
		m, _, _ = handleKeyIncreaseDiffContext(m)
	}
	if got := m.diffContextSize(); got != maxDiffContext {
		t.Fatalf("diff context should clamp to max %d, got %d", maxDiffContext, got)
	}

	for i := 0; i < 20; i++ {
		m, _, _ = handleKeyDecreaseDiffContext(m)
	}
	if got := m.diffContextSize(); got != 0 {
		t.Fatalf("diff context should clamp to 0, got %d", got)
	}
}

func TestVisibleFoldBlocksExcludesChildrenOfCollapsedParent(t *testing.T) {
	r := parser.Resource{
		Address: "helm_release.chart",
		Action:  parser.ActionUpdate,
		RawLines: []string{
			`  ~ resource "helm_release" "chart" {`,
			`      ~ metadata = {`,
			`          ~ values = {`,
			`              nested = true`,
			`            }`,
			`        }`,
		},
	}
	m := Model{
		plan:         &parser.Plan{Resources: []parser.Resource{r}},
		expanded:     map[int]bool{0: true},
		foldedBlocks: make(map[string]bool),
		blockCursor:  -1,
	}
	blocks := findFoldBlocks(r, r.RawLines[1:])
	if len(blocks) != 2 {
		t.Fatalf("expected parent and child folds, got %d", len(blocks))
	}

	m.foldedBlocks[blocks[0].Key] = true
	visible := m.currentFoldBlocks()
	if len(visible) != 1 {
		t.Fatalf("expected only collapsed parent to be visible, got %d", len(visible))
	}
	if visible[0].Key != blocks[0].Key {
		t.Fatalf("expected visible fold to be parent, got %q", visible[0].Key)
	}
}

func TestVisibleFoldBlocksIncludesChildrenOfExpandedParent(t *testing.T) {
	r := parser.Resource{
		Address: "helm_release.chart",
		Action:  parser.ActionUpdate,
		RawLines: []string{
			`  ~ resource "helm_release" "chart" {`,
			`      ~ metadata = {`,
			`          ~ values = {`,
			`              nested = true`,
			`            }`,
			`        }`,
		},
	}
	m := Model{
		plan:         &parser.Plan{Resources: []parser.Resource{r}},
		expanded:     map[int]bool{0: true},
		foldedBlocks: make(map[string]bool),
		blockCursor:  -1,
	}

	visible := m.currentFoldBlocks()
	if len(visible) != 2 {
		t.Fatalf("expected parent and child folds to be visible, got %d", len(visible))
	}
	if visible[0].Start != 0 || visible[1].Start != 1 {
		t.Fatalf("unexpected visible fold order: %#v", visible)
	}
}

func TestSetCurrentScopeFoldsCollapsedResourceScope(t *testing.T) {
	r := parser.Resource{
		Address: "helm_release.chart",
		Action:  parser.ActionUpdate,
		RawLines: []string{
			`  ~ resource "helm_release" "chart" {`,
			`      ~ metadata = {`,
			`          ~ values = {`,
			`              nested = true`,
			`            }`,
			`        }`,
		},
	}
	m := Model{
		plan:         &parser.Plan{Resources: []parser.Resource{r}},
		expanded:     map[int]bool{0: true},
		foldedBlocks: make(map[string]bool),
		blockCursor:  -1,
	}

	if !m.setCurrentScopeFoldsCollapsed(true) {
		t.Fatal("expected resource-scope collapse to apply")
	}
	for _, block := range findFoldBlocks(r, r.RawLines[1:]) {
		if !m.foldedBlocks[block.Key] {
			t.Fatalf("expected fold %q to be collapsed", block.Key)
		}
	}

	if !m.setCurrentScopeFoldsCollapsed(false) {
		t.Fatal("expected resource-scope expand to apply")
	}
	for _, block := range findFoldBlocks(r, r.RawLines[1:]) {
		if m.foldedBlocks[block.Key] {
			t.Fatalf("expected fold %q to be expanded", block.Key)
		}
	}
}

func TestSetCurrentScopeFoldsCollapsedSubBlockScope(t *testing.T) {
	r := parser.Resource{
		Address: "helm_release.chart",
		Action:  parser.ActionUpdate,
		RawLines: []string{
			`  ~ resource "helm_release" "chart" {`,
			`      ~ metadata = {`,
			`          ~ values = {`,
			`              nested = true`,
			`            }`,
			`        }`,
			`      ~ set = {`,
			`          value = true`,
			`        }`,
		},
	}
	m := Model{
		plan:         &parser.Plan{Resources: []parser.Resource{r}},
		expanded:     map[int]bool{0: true},
		foldedBlocks: make(map[string]bool),
		blockCursor:  0,
	}
	blocks := findFoldBlocks(r, r.RawLines[1:])
	if len(blocks) != 3 {
		t.Fatalf("expected metadata, values, and set folds, got %#v", blocks)
	}

	if !m.setCurrentScopeFoldsCollapsed(true) {
		t.Fatal("expected sub-block-scope collapse to apply")
	}
	if !m.foldedBlocks[blocks[0].Key] || !m.foldedBlocks[blocks[1].Key] {
		t.Fatalf("expected selected fold and descendant to collapse: %#v", m.foldedBlocks)
	}
	if m.foldedBlocks[blocks[2].Key] {
		t.Fatalf("did not expect sibling fold to collapse: %#v", m.foldedBlocks)
	}
}

func TestExpandAndCollapseEverythingAffectsAllDisplayedResourcesAndFolds(t *testing.T) {
	resources := []parser.Resource{
		{
			Address: "helm_release.chart",
			Action:  parser.ActionUpdate,
			RawLines: []string{
				`  ~ resource "helm_release" "chart" {`,
				`      ~ metadata = {`,
				`          ~ values = {`,
				`              nested = true`,
				`            }`,
				`        }`,
			},
		},
		{
			Address: "kubectl_manifest.vmagent",
			Action:  parser.ActionUpdate,
			RawLines: []string{
				`  ~ resource "kubectl_manifest" "vmagent" {`,
				`      ~ yaml_body_parsed = <<-EOT`,
				`            spec:`,
				`              replicas: 3`,
				`        EOT`,
			},
		},
	}
	m := Model{
		plan:         &parser.Plan{Resources: resources},
		expanded:     map[int]bool{0: false, 1: false},
		foldedBlocks: make(map[string]bool),
		blockCursor:  1,
	}

	m.expandEverything()
	for idx := range resources {
		if !m.expanded[idx] {
			t.Fatalf("expected resource %d to be expanded", idx)
		}
		for _, block := range findFoldBlocks(resources[idx], resources[idx].RawLines[1:]) {
			if m.foldedBlocks[block.Key] {
				t.Fatalf("expected fold %q to be expanded", block.Key)
			}
		}
	}
	if m.blockCursor != -1 {
		t.Fatalf("expected block cursor to reset after global expand, got %d", m.blockCursor)
	}

	m.collapseEverything()
	for idx := range resources {
		if m.expanded[idx] {
			t.Fatalf("expected resource %d to be collapsed", idx)
		}
		for _, block := range findFoldBlocks(resources[idx], resources[idx].RawLines[1:]) {
			if !m.foldedBlocks[block.Key] {
				t.Fatalf("expected fold %q to be collapsed", block.Key)
			}
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
