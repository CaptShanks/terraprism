package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// fastStyle must always match calling the real lipgloss style's Render
// directly — including for styles like attrOldValueStyle (Strikethrough)
// where lipgloss wraps each run separately and a flat prefix+content+
// suffix concatenation would otherwise be wrong. newFastStyle validates
// this itself and falls back to the real style when needed; this test
// guards that guarantee across representative single-word and
// multi-word/spaced inputs.
func TestFastStyleMatchesLipglossRender(t *testing.T) {
	InitColors()

	cases := []struct {
		name  string
		fast  fastStyle
		style lipgloss.Style
	}{
		{"attrName", fastAttrName, attrNameStyle},
		{"attrOldValue", fastAttrOldValue, attrOldValueStyle},
		{"attrNewValue", fastAttrNewValue, attrNewValueStyle},
		{"attrComputed", fastAttrComputed, attrComputedStyle},
		{"muted", fastMuted, mutedColor},
	}
	inputs := []string{"hello", `"quoted value"`, "value with spaces", ""}

	for _, tc := range cases {
		for _, input := range inputs {
			want := tc.style.Render(input)
			got := tc.fast.Render(input)
			if got != want {
				t.Errorf("%s.Render(%q): fast=%q, want %q", tc.name, input, got, want)
			}
		}
	}
}
