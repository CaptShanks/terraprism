package tui

import (
	"strings"
	"testing"
)

func TestTryPrettyPrintJSONParsesAndIndentsAString(t *testing.T) {
	got, ok := TryPrettyPrintJSON(`{"b":2,"a":1}`)
	if !ok {
		t.Fatalf("expected a JSON object string to parse")
	}
	// json.Marshal sorts map keys alphabetically regardless of source order.
	want := "{\n  \"a\": 1,\n  \"b\": 2\n}"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTryPrettyPrintJSONMarshalsAContainerValueDirectly(t *testing.T) {
	// The shape a whole-subtree-sensitive attribute's raw Old/New
	// actually carries (see tfplan.buildAttribute) -- no parse step
	// needed, just marshal it.
	got, ok := TryPrettyPrintJSON(map[string]interface{}{"user": "admin"})
	if !ok {
		t.Fatalf("expected a map[string]interface{} to marshal directly")
	}
	if !strings.Contains(got, `"user": "admin"`) {
		t.Fatalf("got %q, want it to contain the pretty-printed key", got)
	}

	got, ok = TryPrettyPrintJSON([]interface{}{"a", "b"})
	if !ok {
		t.Fatalf("expected a []interface{} to marshal directly")
	}
	if !strings.Contains(got, `"a"`) || !strings.Contains(got, `"b"`) {
		t.Fatalf("got %q, want both list elements", got)
	}
}

func TestTryPrettyPrintJSONRejectsBareScalars(t *testing.T) {
	// A bare JSON scalar parses fine but pretty-printing it is still one
	// line -- no benefit, so it's deliberately excluded rather than
	// wrapped in heredoc ceremony for nothing.
	for _, v := range []any{`"just a string"`, `123`, `true`, `null`} {
		if _, ok := TryPrettyPrintJSON(v); ok {
			t.Errorf("TryPrettyPrintJSON(%v) = ok, want rejected (bare scalar)", v)
		}
	}
}

func TestTryPrettyPrintJSONRejectsNonJSONAndOtherTypes(t *testing.T) {
	cases := []any{
		"not json at all",
		"",
		nil,
		42, // a real (non-string) scalar, not the JSON-encoding of one
		true,
		[]string{"not", "the", "decoded", "shape"},
	}
	for _, v := range cases {
		if _, ok := TryPrettyPrintJSON(v); ok {
			t.Errorf("TryPrettyPrintJSON(%#v) = ok, want rejected", v)
		}
	}
}

func TestIsJSONContainerAttrChecksBothOldAndNew(t *testing.T) {
	t.Run("neither side is JSON", func(t *testing.T) {
		if isJSONContainerAttr(leafSensitive("x", "update", "plain old", "plain new")) {
			t.Fatalf("expected false when neither Old nor New is JSON-shaped")
		}
	})
	t.Run("only New is JSON", func(t *testing.T) {
		if !isJSONContainerAttr(leafSensitive("x", "update", "plain old", `{"a":1}`)) {
			t.Fatalf("expected true when New alone is JSON-shaped")
		}
	})
}
