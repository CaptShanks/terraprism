package tui

import (
	"strings"

	"github.com/CaptShanks/terraprism/internal/foldtree"
	"github.com/CaptShanks/terraprism/internal/tfplan"
)

// rowKind identifies what a foldtree row represents.
type rowKind int

const (
	rowResource rowKind = iota
	rowLeaf
	rowContainerHeader
	rowMultilineHeader
	rowMultilineBody  // synthetic child: the diffed body block
	rowClosingBracket // synthetic child: "}" / "]"
	rowEOT            // synthetic child: closing "EOT"
	rowUnchangedNote  // synthetic: one collapsed run of ActionNoOp attrs
	rowSensitive
	rowUserdata
)

// rowInfo carries everything the flat-loop renderer needs for one row,
// attached directly to its foldtree.Node/Row as Payload. text is the
// pre-rendered, unselected content for kinds whose text doesn't depend
// on live collapse state (everything except rowResource/
// rowContainerHeader/rowMultilineHeader, which render fresh each time
// from attr/resource since their text changes with the current
// collapsed/expanded state).
type rowInfo struct {
	kind     rowKind
	resource tfplan.Resource
	attr     tfplan.Attribute
	text     string
	keyed    bool // for rowContainerHeader/rowMultilineHeader, rendered fresh each call
}

// SearchText implements foldtree.Searchable for rowResource payloads,
// matching what performSearch has always searched over: a resource's
// address, type, and name.
func (info rowInfo) SearchText() string {
	r := info.resource
	return r.Address + " " + r.Type + " " + r.Name
}

// buildResourceNode converts one resource's attribute tree into a
// foldtree.Node rooted at the resource itself.
//
// This mirrors renderAttributeTree's exact branching (noop-run
// collapsing, userdata, container, sensitive, multiline, leaf) so that a
// row's declared Height is always exactly what will be rendered: for any
// row whose content doesn't depend on live collapse/selection state, the
// text is rendered once, right here, by calling the same render
// functions the old recursive renderer used, and Height is derived by
// counting the newlines in that exact string — never a parallel
// estimate. That's what prevents the "declared height disagrees with
// actual rendered height" bug class from creeping back in at the height
// level, the same way foldtree itself prevents it at the cursor/scroll
// level.
func (m *Model) buildResourceNode(r tfplan.Resource) foldtree.Node {
	return foldtree.Node{
		ID:          r.Address,
		Height:      1,
		Collapsible: true,
		Payload:     rowInfo{kind: rowResource, resource: r},
		Children:    m.buildAttributeNodes(r.Address, r.Attributes, 1, true),
	}
}

// buildAttributeNodes recursively converts attrs into foldtree.Nodes.
// depth/keyed match renderAttributeTree's own parameters exactly (keyed
// is true for resource-level attributes and map entries, false for
// positional list elements).
func (m *Model) buildAttributeNodes(address string, attrs []tfplan.Attribute, depth int, keyed bool) []foldtree.Node {
	maxWidth := m.treeView.Width()
	indent := strings.Repeat("  ", depth)
	var nodes []foldtree.Node

	for i := 0; i < len(attrs); i++ {
		attr := attrs[i]

		if attr.Action == tfplan.ActionNoOp {
			run := 1
			for i+run < len(attrs) && attrs[i+run].Action == tfplan.ActionNoOp {
				run++
			}
			id := foldKey(address, attr.Path) + "#noop"
			info := rowInfo{kind: rowUnchangedNote, text: indent + fastMuted.Render(unchangedHiddenNote(run))}
			nodes = append(nodes, foldtree.Node{ID: id, Height: 1, Payload: info})
			i += run - 1
			continue
		}

		// Checked before tryRenderUserdataAttr/isContainerAttr/
		// isMultilineStringAttr: those all inspect attr.Old/New, which
		// now carry the real value for a sensitive attribute too (see
		// tfplan.buildAttribute). Gating on Sensitive first, rather than
		// relying on isContainerAttr's own "!a.Sensitive" guard and the
		// others incidentally failing on a redacted nil value, keeps
		// redaction unconditional regardless of what those checks do.
		//
		// The one deliberate exception: a revealed value that's either
		// already multi-line or JSON-shaped (a minified JSON blob has no
		// literal newlines of its own to trip isMultilineStringAttr, but
		// pretty-printing it would produce plenty -- see
		// sensitiveRevealsAsHeredoc) falls through to the
		// isMultilineStringAttr block below instead of this flat row, so
		// it gets the same foldable, per-line diff every other multi-line
		// value gets -- flattening it into one %q-escaped line here would
		// turn embedded newlines into literal "\n" text (or, for JSON,
		// leave it as one unreadable blob) and make it unreadable.
		// renderMultilineStringBody still redacts it itself whenever
		// revealSensitive is off, so this is never reachable while the
		// value is still hidden.
		revealsAsHeredoc := m.revealSensitive && sensitiveRevealsAsHeredoc(attr)
		if attr.Sensitive && !revealsAsHeredoc {
			id := foldKey(address, attr.Path)
			var text string
			if m.revealSensitive {
				text = indent + actionPrefixSymbol(attr.Action) + " " + renderRevealedSensitiveValue(attr, keyed)
			} else {
				text = indent + actionPrefixSymbol(attr.Action) + " " + renderKeyValue(attr, keyed)
			}
			info := rowInfo{kind: rowSensitive, attr: attr, text: text}
			nodes = append(nodes, foldtree.Node{ID: id, Height: 1, Payload: info})
			continue
		}

		if keyed {
			if decoded, ok := m.tryRenderUserdataAttr(attr, indent, maxWidth); ok {
				id := foldKey(address, attr.Path)
				info := rowInfo{kind: rowUserdata, attr: attr, text: decoded}
				nodes = append(nodes, foldtree.Node{ID: id, Height: strings.Count(decoded, "\n") + 1, Payload: info})
				continue
			}
		}

		if isContainerAttr(attr) {
			id := foldKey(address, attr.Path)

			childKeyed := attr.Kind == tfplan.KindMap
			children := m.buildAttributeNodes(address, attr.Children, depth+1, childKeyed)

			closeID := id + "#close"
			closeInfo := rowInfo{kind: rowClosingBracket, attr: attr, text: closingBraceLine(indent, attr.Kind)}
			children = append(children, foldtree.Node{ID: closeID, Height: 1, Payload: closeInfo})

			info := rowInfo{kind: rowContainerHeader, attr: attr, keyed: keyed}
			nodes = append(nodes, foldtree.Node{ID: id, Height: 1, Collapsible: true, Payload: info, Children: children})
			continue
		}

		if isMultilineStringAttr(attr) || sensitiveRevealsAsHeredoc(attr) {
			id := foldKey(address, attr.Path)

			body := m.renderMultilineStringBody(attr, indent, maxWidth)
			bodyID := id + "#body"
			bodyInfo := rowInfo{kind: rowMultilineBody, attr: attr, text: body}

			eotID := id + "#eot"
			eotInfo := rowInfo{kind: rowEOT, text: indent + fastMuted.Render("EOT")}

			children := []foldtree.Node{
				{ID: bodyID, Height: strings.Count(body, "\n") + 1, Payload: bodyInfo},
				{ID: eotID, Height: 1, Payload: eotInfo},
			}
			info := rowInfo{kind: rowMultilineHeader, attr: attr, keyed: keyed}
			nodes = append(nodes, foldtree.Node{ID: id, Height: 1, Collapsible: true, Payload: info, Children: children})
			continue
		}

		// Plain leaf -- also the fallback for an empty container (a map
		// or list with zero children), which isContainerAttr excludes.
		id := foldKey(address, attr.Path)
		row := m.renderLeafRow(indent, attr, keyed, maxWidth)
		info := rowInfo{kind: rowLeaf, attr: attr, text: row}
		nodes = append(nodes, foldtree.Node{ID: id, Height: strings.Count(row, "\n") + 1, Payload: info})
	}

	return nodes
}
