package tui

import (
	"encoding/json"
	"strings"

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
// Always false for a non-sensitive attribute -- used only to decide
// whether a sensitive value may be shown at all once revealed (see
// buildAttributeNodes); it has nothing to do with whether an *ordinary*
// attribute gets JSON pretty-printing, which is unconditional (see
// multilineDisplayText) since there's no redaction to gate there.
func sensitiveRevealsAsHeredoc(attr tfplan.Attribute) bool {
	if !attr.Sensitive {
		return false
	}
	return isMultilineStringAttr(attr) || isJSONContainerAttr(attr)
}

// multilineDisplayText returns the text to diff for one side (Old or
// New) of a value being rendered as a foldable multi-line block: the
// value as-is if it's already a string containing a real line break (an
// ordinary multi-line string is never touched further, so an
// already-pretty JSON blob's own key order and formatting are never
// disturbed), otherwise its pretty-printed form if it's JSON-shaped (a
// minified blob, or a whole-subtree-sensitive container's raw
// map/list, has no line structure of its own to diff), otherwise the
// plain string unchanged (possibly "" for a non-string, non-JSON value).
//
// Deliberately not gated on Sensitive: once a value has been decided to
// render as a multi-line block at all (isMultilineStringAttr or
// isJSONContainerAttr, checked by the caller), pretty-printing it if
// it's JSON-shaped is always an improvement, sensitive or not.
func multilineDisplayText(v any) string {
	if s, ok := v.(string); ok && strings.Contains(s, "\n") {
		return s
	}
	if pretty, ok := TryPrettyPrintJSON(v); ok {
		return pretty
	}
	s, _ := v.(string)
	return s
}

// jsonPrettyPrintApplies reports whether rendering this attribute's diff
// body actually involves pretty-printing a JSON value on at least one
// side -- used for the "(JSON, pretty-printed)" header cue. Stricter
// than isJSONContainerAttr: an already-multi-line value that also
// happens to be valid JSON is shown as-is by multilineDisplayText (its
// formatting is never touched), so it must not claim to be
// pretty-printed.
func jsonPrettyPrintApplies(attr tfplan.Attribute) bool {
	sideApplies := func(v any) bool {
		if s, ok := v.(string); ok && strings.Contains(s, "\n") {
			return false
		}
		_, ok := TryPrettyPrintJSON(v)
		return ok
	}
	return sideApplies(attr.Old) || sideApplies(attr.New)
}
