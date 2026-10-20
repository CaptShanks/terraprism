package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/CaptShanks/terraprism/internal/tfplan"
)

// largePlanWithBlobs builds n resources, each with a handful of plain
// attributes plus one big multiline-string attribute (like a Helm
// values.yaml or a cloud-init script) -- the shape that makes
// rebuildTree's eager, always-recompute-every-displayed-resource design
// expensive: every resize or diffContext change re-runs diff/wrap work
// for every attribute of every displayed resource, including ones still
// collapsed, since a node's height must be correct before it's ever
// expanded.
func largePlanWithBlobs(resourceCount, blobLines int) *tfplan.Plan {
	oldBlob := make([]string, blobLines)
	newBlob := make([]string, blobLines)
	for i := 0; i < blobLines; i++ {
		oldBlob[i] = fmt.Sprintf("  setting_%d: value-%d", i, i)
		newBlob[i] = fmt.Sprintf("  setting_%d: value-%d", i, i)
	}
	newBlob[blobLines/2] = "  setting_changed: new-value"
	oldValues := "values:\n" + strings.Join(oldBlob, "\n")
	newValues := "values:\n" + strings.Join(newBlob, "\n")

	resources := make([]tfplan.Resource, resourceCount)
	for i := range resources {
		address := fmt.Sprintf("helm_release.chart_%d", i)
		resources[i] = tfplan.Resource{
			Address: address,
			Action:  tfplan.ActionUpdate,
			Attributes: withPaths([]tfplan.Attribute{
				leaf("chart", tfplan.ActionNoOp, tfplan.KindString, "my-chart", "my-chart"),
				leaf("version", tfplan.ActionUpdate, tfplan.KindString, "1.0.0", "1.1.0"),
				mapBlock("set", tfplan.ActionUpdate,
					leaf("replicaCount", tfplan.ActionUpdate, tfplan.KindNumber, "2", "3"),
					leaf("image_tag", tfplan.ActionNoOp, tfplan.KindString, "v1", "v1"),
				),
				leaf("values", tfplan.ActionUpdate, tfplan.KindString, oldValues, newValues),
			}, ""),
		}
	}
	return &tfplan.Plan{Resources: resources}
}

// BenchmarkRebuildTree measures the cost of the full adapter rebuild
// (buildResourceNode for every displayed resource + nav.SetTree +
// applyDefaultCollapse) -- what runs on every terminal resize and every
// diffContext +/- keystroke.
func BenchmarkRebuildTree(b *testing.B) {
	plan := largePlanWithBlobs(50, 60)
	m := NewModel(plan, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm := model.(Model)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.rebuildTree()
	}
}

// BenchmarkResize measures a full resize round trip through Update(),
// end to end -- rebuildTree plus render() plus viewport sync -- to
// approximate the actual per-keystroke cost a user would feel while
// interactively resizing their terminal.
func BenchmarkResize(b *testing.B) {
	plan := largePlanWithBlobs(50, 60)
	m := NewModel(plan, "")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	mm := model.(Model)

	widths := []int{120, 121}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := widths[i%2]
		model, _ := mm.Update(tea.WindowSizeMsg{Width: w, Height: 40})
		mm = model.(Model)
	}
}
