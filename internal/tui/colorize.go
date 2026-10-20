package tui

import (
	"encoding/json"
	"fmt"

	"github.com/CaptShanks/terraprism/internal/tfplan"
)

// actionPrefixSymbol returns the +/-/~ (or blank) diff-line symbol for an
// attribute-level action.
func actionPrefixSymbol(action tfplan.Action) string {
	switch action {
	case tfplan.ActionCreate:
		return createSymbol
	case tfplan.ActionDelete:
		return destroySymbol
	case tfplan.ActionUpdate:
		return updateSymbol
	default:
		return " "
	}
}

// renderScalarText renders a decoded JSON scalar as displayable text,
// quoting strings so they read like HCL literals.
func renderScalarText(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		if t {
			return "true"
		}
		return "false"
	case json.Number:
		return t.String()
	case string:
		return fmt.Sprintf("%q", t)
	case []interface{}, map[string]interface{}:
		// Only reachable for a whole-subtree-sensitive attribute revealed
		// via renderRevealedSensitiveValue -- an ordinary container is
		// always walked into Children and never reaches this function
		// with a raw map/list value.
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprintf("%v", t)
		}
		return string(b)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// renderLeafValue renders a leaf attribute's value (no key, no line
// prefix), applying sensitive/computed styling and old->new diffing.
// Unchanged (no-op) values are always muted rather than colored by type —
// they're just context, and coloring them by type (e.g. red for null,
// yellow for numbers) made them visually compete with attributes that
// actually changed. In practice renderAttributeTree filters no-op leaves
// out into a "(N unchanged attributes hidden)" summary before they ever
// reach here (matching Terraform CLI's own convention), so this case is
// a defensive fallback, not the primary path.
//
// Uses the fastStyle (precomputed-ANSI) variants rather than calling
// lipgloss.Style.Render directly: this is the hottest call site in the
// whole renderer (invoked once per leaf attribute), and a plan with a
// few thousand attributes made the per-call cost of lipgloss's full
// style-resolution pipeline add up to a user-visible stall on every
// keystroke.
func renderLeafValue(attr tfplan.Attribute) string {
	if attr.Sensitive {
		return fastSensitive.Render("(sensitive value)")
	}

	switch attr.Action {
	case tfplan.ActionCreate:
		if attr.Computed {
			return fastAttrComputed.Render("(known after apply)")
		}
		return fastAttrNewValue.Render(renderScalarText(attr.New))
	case tfplan.ActionDelete:
		return fastAttrOldValue.Render(renderScalarText(attr.Old))
	case tfplan.ActionNoOp:
		return fastMuted.Render(renderScalarText(attr.New))
	default: // update
		oldText := fastAttrOldValue.Render(renderScalarText(attr.Old))
		var newText string
		if attr.Computed {
			newText = fastAttrComputed.Render("(known after apply)")
		} else {
			newText = fastAttrNewValue.Render(renderScalarText(attr.New))
		}
		return oldText + " → " + newText
	}
}

// renderKeyValue renders "name = value" for a keyed leaf attribute, or
// just "value" when keyed is false (a bare, positional list element).
func renderKeyValue(attr tfplan.Attribute, keyed bool) string {
	if !keyed {
		return renderLeafValue(attr)
	}
	return fastAttrName.Render(attr.Name) + " = " + renderLeafValue(attr)
}

// renderRevealedSensitiveValue renders a sensitive attribute's real
// value once the user has opted in via the 'x' hotkey, using the same
// create/delete/update diff coloring as an ordinary attribute so the
// actual change is visible. Prefixed with a plain-text marker (not an
// emoji) so it stays visually distinguishable from a non-sensitive
// attribute at a glance, without risking lipgloss's Width() miscounting
// a double-width glyph the way an emoji marker could.
func renderRevealedSensitiveValue(attr tfplan.Attribute, keyed bool) string {
	revealed := attr
	revealed.Sensitive = false
	return fastSensitive.Render("(revealed) ") + renderKeyValue(revealed, keyed)
}

// containerBrackets returns the open/close delimiters for a container
// attribute's Kind.
func containerBrackets(kind tfplan.ValueKind) (open, close string) {
	if kind == tfplan.KindList {
		return "[", "]"
	}
	return "{", "}"
}
