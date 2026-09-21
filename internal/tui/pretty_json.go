package tui

import (
	"encoding/json"

	"github.com/CaptShanks/terraprism/internal/tfplan"
)

// TryPrettyPrintJSON attempts to render v as indented, multi-line JSON
// text suitable for line-by-line diffing. v may be a JSON-encoded
// string (parsed then re-marshaled), or an already-decoded container
// value -- map[string]interface{}/[]interface{}, the shape a
// whole-subtree-sensitive attribute's raw Old/New carries (see
// tfplan.buildAttribute) -- marshaled directly with no parse step.
//
// Returns ("", false) for anything that isn't a JSON object or array:
// nil, a plain scalar, or a string that fails to parse. A bare JSON
// scalar (e.g. the string "true" or "123") is deliberately excluded
// too -- pretty-printing it would still be one line, so it gets no
// benefit from the heredoc treatment this exists to feed and would
// just look like unnecessary ceremony.
func TryPrettyPrintJSON(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return "", false
		}
		var generic any
		if err := json.Unmarshal([]byte(t), &generic); err != nil {
			return "", false
		}
		return marshalIndentIfContainer(generic)
	case map[string]interface{}, []interface{}:
		return marshalIndentIfContainer(t)
	default:
		return "", false
	}
}

func marshalIndentIfContainer(v any) (string, bool) {
	switch v.(type) {
	case map[string]interface{}, []interface{}:
	default:
		return "", false
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", false
	}
	return string(b), true
}

// isJSONContainerAttr reports whether attr's real Old or New value is
// JSON-shaped -- either already a decoded container (a
// whole-subtree-sensitive map/list) or a string that parses as a JSON
// object/array -- and would therefore benefit from being pretty-printed
// and diffed line by line instead of shown as one unreadable blob.
func isJSONContainerAttr(attr tfplan.Attribute) bool {
	if _, ok := TryPrettyPrintJSON(attr.Old); ok {
		return true
	}
	if _, ok := TryPrettyPrintJSON(attr.New); ok {
		return true
	}
	return false
}

// sensitiveRevealsAsHeredoc reports whether a *sensitive* attribute,
// once revealed, should get the same foldable line-by-line diff a
// genuinely multi-line string gets, rather than the flat single-line
// reveal -- either because it already contains a literal newline, or
// because its real value is JSON-shaped and pretty-printing it would
// produce one (a minified JSON blob has no literal newlines of its own,
// so isMultilineStringAttr alone would never catch it).
//
// Always false for a non-sensitive attribute: an ordinary JSON-shaped
// attribute keeps its existing flat rendering -- this is deliberately
// scoped to the sensitive-reveal path only, not a general "pretty-print
// any JSON string" feature.
func sensitiveRevealsAsHeredoc(attr tfplan.Attribute) bool {
	if !attr.Sensitive {
		return false
	}
	return isMultilineStringAttr(attr) || isJSONContainerAttr(attr)
}
