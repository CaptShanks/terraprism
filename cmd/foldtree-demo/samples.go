package main

import (
	"fmt"

	"github.com/CaptShanks/terraprism/internal/foldtree"
)

// content is the demo's own per-node display data. foldtree.Node
// deliberately carries no content of its own, so the demo keeps a
// parallel registry keyed by ID.
type content struct {
	lines []string
}

func leafNode(reg map[string]content, id, label string) foldtree.Node {
	reg[id] = content{lines: []string{label}}
	return foldtree.Node{ID: id, Height: 1}
}

func blockNode(reg map[string]content, id, label string, children ...foldtree.Node) foldtree.Node {
	reg[id] = content{lines: []string{label}}
	return foldtree.Node{ID: id, Height: 1, Collapsible: true, Children: children}
}

func multilineNode(reg map[string]content, id string, lines []string) foldtree.Node {
	reg[id] = content{lines: lines}
	return foldtree.Node{ID: id, Height: len(lines)}
}

func zeroHeightNode(reg map[string]content, id, label string) foldtree.Node {
	reg[id] = content{lines: nil} // nothing rendered, but label kept for reference
	_ = label
	return foldtree.Node{ID: id, Height: 0}
}

type variant struct {
	name  string
	build func() ([]foldtree.Node, map[string]content)
}

var variants = []variant{
	{"flat list (30 rows)", buildFlat},
	{"nested resource tree", buildNested},
	{"deep chain (40 levels)", buildChain},
	{"wide fanout (300 siblings)", buildWide},
	{"multiline row (taller than the viewport)", buildMultiline},
	{"faulty: zero-height rows", buildFaulty},
	{"large: many resources with big text blobs", buildLargeWithBlobs},
}

func buildFlat() ([]foldtree.Node, map[string]content) {
	reg := map[string]content{}
	nodes := make([]foldtree.Node, 30)
	for i := range nodes {
		nodes[i] = leafNode(reg, fmt.Sprintf("row-%d", i), fmt.Sprintf("Row %d", i))
	}
	return nodes, reg
}

func buildNested() ([]foldtree.Node, map[string]content) {
	reg := map[string]content{}

	mkResource := func(name string, nics int) foldtree.Node {
		tags := blockNode(reg, name+".tags", "tags",
			leafNode(reg, name+".tags.Name", fmt.Sprintf(`Name = "%s"`, name)),
			leafNode(reg, name+".tags.Env", `Environment = "prod"`),
		)
		var nicNodes []foldtree.Node
		for i := 0; i < nics; i++ {
			nicID := fmt.Sprintf("%s.nic[%d]", name, i)
			nicNodes = append(nicNodes, blockNode(reg, nicID, fmt.Sprintf("network_interface[%d]", i),
				leafNode(reg, nicID+".subnet", fmt.Sprintf(`subnet_id  = "subnet-%d"`, i)),
				leafNode(reg, nicID+".ip", fmt.Sprintf(`private_ip = "10.0.%d.5"`, i)),
			))
		}
		nicsBlock := blockNode(reg, name+".nics", "network_interfaces", nicNodes...)
		return blockNode(reg, name, fmt.Sprintf("resource %q", name), tags, nicsBlock,
			leafNode(reg, name+".ami", `ami = "ami-0123456789"`),
		)
	}

	return []foldtree.Node{
		mkResource("aws_instance.web", 2),
		mkResource("aws_instance.db", 1),
		leafNode(reg, "output.url", `output "url" = "https://example.com"`),
	}, reg
}

func buildChain() ([]foldtree.Node, map[string]content) {
	reg := map[string]content{}
	const depth = 40

	var inner foldtree.Node
	for i := depth - 1; i >= 0; i-- {
		id := fmt.Sprintf("level-%d", i)
		label := fmt.Sprintf("Level %d", i)
		if i == depth-1 {
			inner = leafNode(reg, id, label)
			continue
		}
		inner = blockNode(reg, id, label, inner)
	}
	return []foldtree.Node{inner}, reg
}

func buildWide() ([]foldtree.Node, map[string]content) {
	reg := map[string]content{}
	nodes := make([]foldtree.Node, 300)
	for i := range nodes {
		nodes[i] = leafNode(reg, fmt.Sprintf("item-%d", i), fmt.Sprintf("Item %d", i))
	}
	return nodes, reg
}

func buildMultiline() ([]foldtree.Node, map[string]content) {
	reg := map[string]content{}
	desc := multilineNode(reg, "description", []string{
		"description:",
		"  This row is 15 lines tall -- taller than most terminal",
		"  viewports. Navigate onto it with j/k and watch the",
		"  viewport align to its TOP rather than trying (and",
		"  failing) to fit the whole thing, or centering it.",
		"  ",
		"  line 6",
		"  line 7",
		"  line 8",
		"  line 9",
		"  line 10",
		"  line 11",
		"  line 12",
		"  line 13",
		"  line 14 (last line)",
	})
	return []foldtree.Node{
		leafNode(reg, "before-1", "Row before-1"),
		leafNode(reg, "before-2", "Row before-2"),
		desc,
		leafNode(reg, "after-1", "Row after-1"),
		leafNode(reg, "after-2", "Row after-2"),
	}, reg
}

func buildFaulty() ([]foldtree.Node, map[string]content) {
	reg := map[string]content{}
	return []foldtree.Node{
		leafNode(reg, "a", "Row a"),
		zeroHeightNode(reg, "ghost-1", "zero-height row 1"),
		zeroHeightNode(reg, "ghost-2", "zero-height row 2"),
		leafNode(reg, "b", "Row b (watch: two ghost rows sat between a and b above, invisibly)"),
		leafNode(reg, "c", "Row c"),
	}, reg
}

// buildLargeWithBlobs builds a realistically large tree -- several
// hundred rows, well beyond a single screen -- with big multi-line text
// blobs (fake Helm values.yaml and release notes) mixed in among
// ordinary short attributes, the shape terraprism's real usage actually
// looks like (Helm chart diffs), rather than the small isolated
// multiline example above.
func buildLargeWithBlobs() ([]foldtree.Node, map[string]content) {
	reg := map[string]content{}
	const resourceCount = 6

	var resources []foldtree.Node
	for i := 0; i < resourceCount; i++ {
		name := fmt.Sprintf("helm_release.chart_%d", i)

		values := multilineNode(reg, name+".values", yamlBlob(i, 35+i*6))
		notes := multilineNode(reg, name+".notes", notesBlob(i, 15))

		set := blockNode(reg, name+".set", "set",
			leafNode(reg, name+".set.replicas", fmt.Sprintf("replicaCount = %d", i+1)),
			leafNode(reg, name+".set.tag", `image.tag = "v2.3.1"`),
			leafNode(reg, name+".set.pullPolicy", `image.pullPolicy = "IfNotPresent"`),
		)

		resources = append(resources, blockNode(reg, name, fmt.Sprintf("resource %q", name),
			leafNode(reg, name+".chart", `chart   = "my-chart"`),
			leafNode(reg, name+".version", fmt.Sprintf(`version = "1.%d.0"`, i)),
			set,
			values,
			notes,
		))
	}
	return resources, reg
}

func yamlBlob(seed, lines int) []string {
	out := make([]string, 0, lines+1)
	out = append(out, "values:")
	for i := 0; i < lines; i++ {
		out = append(out, fmt.Sprintf("  chart_%d_setting_%d: \"value-%d\"", seed, i, i))
	}
	return out
}

func notesBlob(seed, lines int) []string {
	out := make([]string, 0, lines+1)
	out = append(out, "NOTES:")
	for i := 0; i < lines; i++ {
		out = append(out, fmt.Sprintf("  release %d is now deployed, step %d of %d", seed, i+1, lines))
	}
	return out
}
