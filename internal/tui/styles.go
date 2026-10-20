package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
)

type fastStyleMode int

const (
	fastStyleModeFallback fastStyleMode = iota // call fallback.Render directly
	fastStyleModeSimple                        // prefix + s + suffix
	fastStyleModePerRune                       // wrap each rune individually; different wrap for whitespace
)

// fastStyle precomputes a style's ANSI escape codes once (via real
// lipgloss.Style.Render calls), so rendering many small tokens with the
// same style — the common case when coloring thousands of plan attribute
// rows — is cheap string concatenation instead of re-running lipgloss's
// full per-call style resolution (~25 property checks, line splitting,
// width calculation) every time.
//
// Strikethrough/Underline (without their *Spaces counterpart) make
// lipgloss wrap each *rune* individually — differently for whitespace vs
// not — rather than the whole string once; fastStyleModePerRune
// replicates that exactly. newFastStyle always validates its derived
// fast path against a multi-word probe string and falls back to the
// real style's Render when it doesn't match byte-for-byte, so this is
// always at least as correct as calling style.Render directly.
type fastStyle struct {
	prefix      string
	suffix      string
	spacePrefix string
	spaceSuffix string
	fallback    lipgloss.Style
	mode        fastStyleMode
}

const fastStyleProbe = "sample value with spaces"

func newFastStyle(s lipgloss.Style) fastStyle {
	fs := fastStyle{fallback: s}

	const marker = "\x00"
	rendered := s.Render(marker)
	idx := strings.Index(rendered, marker)
	if idx < 0 {
		return fs
	}
	fs.prefix, fs.suffix = rendered[:idx], rendered[idx+len(marker):]

	if fs.prefix+fastStyleProbe+fs.suffix == s.Render(fastStyleProbe) {
		fs.mode = fastStyleModeSimple
		return fs
	}

	// Try replicating lipgloss's per-rune whitespace-aware wrapping
	// (used for Strikethrough/Underline): derive the whitespace variant
	// from rendering a lone space, then validate against the full probe.
	spaceRendered := s.Render(" ")
	spIdx := strings.Index(spaceRendered, " ")
	if spIdx >= 0 {
		fs.spacePrefix, fs.spaceSuffix = spaceRendered[:spIdx], spaceRendered[spIdx+1:]
		fs.mode = fastStyleModePerRune
		if fs.renderPerRune(fastStyleProbe) == s.Render(fastStyleProbe) {
			return fs
		}
	}

	fs.mode = fastStyleModeFallback
	return fs
}

func (f fastStyle) Render(s string) string {
	switch f.mode {
	case fastStyleModeSimple:
		if f.prefix == "" && f.suffix == "" {
			return s
		}
		return f.prefix + s + f.suffix
	case fastStyleModePerRune:
		return f.renderPerRune(s)
	default:
		return f.fallback.Render(s)
	}
}

// renderPerRune replicates lipgloss's whitespace-aware per-rune wrapping
// (see style.go's useSpaceStyler): every rune gets its own prefix/suffix,
// using the whitespace variant for whitespace runes.
func (f fastStyle) renderPerRune(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 3)
	for _, r := range s {
		if unicode.IsSpace(r) {
			b.WriteString(f.spacePrefix)
			b.WriteRune(r)
			b.WriteString(f.spaceSuffix)
			continue
		}
		b.WriteString(f.prefix)
		b.WriteRune(r)
		b.WriteString(f.suffix)
	}
	return b.String()
}

// Color palette - bound to terminal ANSI slots so the app follows the user's shell theme.
var (
	createColor   lipgloss.Color
	destroyColor  lipgloss.Color
	updateColor   lipgloss.Color
	replaceColor  lipgloss.Color
	readColor     lipgloss.Color
	selectedBg    lipgloss.Color
	headerColor   lipgloss.Color
	mutedColorVal lipgloss.Color
	textColor     lipgloss.Color
	computedColor lipgloss.Color
)

// ANSI 16-color slot assignments. Terminals remap these per theme, so the app
// inherits whatever palette the user has set in their shell.
var ansiPalette = map[string]string{
	"green":   "2",  // create
	"red":     "1",  // destroy
	"yellow":  "3",  // update
	"magenta": "5",  // replace
	"cyan":    "6",  // read / computed
	"blue":    "4",  // header / info
	"gray":    "8",  // muted / selection bg (bright black)
	"text":    "",   // inherit terminal default foreground
}

// IsLightBackground is retained for backward compatibility. With ANSI-bound
// colors the terminal owns the palette, so this no longer drives styling.
func IsLightBackground() bool {
	return false
}

func init() {
	InitColors()
}

// InitColors assigns the ANSI palette and (re)builds the lipgloss styles.
func InitColors() {
	createColor = lipgloss.Color(ansiPalette["green"])
	destroyColor = lipgloss.Color(ansiPalette["red"])
	updateColor = lipgloss.Color(ansiPalette["yellow"])
	replaceColor = lipgloss.Color(ansiPalette["magenta"])
	readColor = lipgloss.Color(ansiPalette["cyan"])
	headerColor = lipgloss.Color(ansiPalette["blue"])
	mutedColorVal = lipgloss.Color(ansiPalette["gray"])
	selectedBg = lipgloss.Color(ansiPalette["gray"])
	textColor = lipgloss.Color(ansiPalette["text"])
	computedColor = lipgloss.Color(ansiPalette["cyan"])
	initStyles()
}

// SetDarkPalette is kept for backward compatibility. Colors now follow the
// terminal's ANSI palette, so explicit dark/light selection is unnecessary.
func SetDarkPalette() { InitColors() }

// SetLightPalette is kept for backward compatibility. See SetDarkPalette.
func SetLightPalette() { InitColors() }

// Styles - initialized after colors are set
var (
	appStyle             lipgloss.Style
	headerStyle          lipgloss.Style
	summaryStyle         lipgloss.Style
	resourceCreateStyle  lipgloss.Style
	resourceDestroyStyle lipgloss.Style
	resourceUpdateStyle  lipgloss.Style
	resourceReplaceStyle lipgloss.Style
	resourceReadStyle    lipgloss.Style
	attrNameStyle        lipgloss.Style
	attrOldValueStyle    lipgloss.Style
	attrNewValueStyle    lipgloss.Style
	attrComputedStyle    lipgloss.Style
	mutedColor           lipgloss.Style
	helpStyle            lipgloss.Style
	searchStyle          lipgloss.Style
	matchStyle           lipgloss.Style
)

// Action symbols - set after colors
var (
	createSymbol       string
	destroySymbol      string
	updateSymbol       string
	replaceSymbol      string
	readSymbol         string
	forgetSymbol       string
	expandedIndicator  string
	collapsedIndicator string
)

// Fast (precomputed-ANSI) equivalents of the styles above, for the hot
// per-attribute-row rendering path (internal/tui/colorize.go, model.go's
// renderAttributeTree and helpers, print.go). See fastStyle.
var (
	fastAttrName     fastStyle
	fastAttrOldValue fastStyle
	fastAttrNewValue fastStyle
	fastAttrComputed fastStyle
	fastMuted        fastStyle
	fastSensitive    fastStyle
	fastCreate       fastStyle
	fastDestroy      fastStyle
)

func initStyles() {
	// App container
	appStyle = lipgloss.NewStyle().
		Padding(1, 2)

	// Header
	headerStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(headerColor).
		MarginBottom(1)

	// Summary line
	summaryStyle = lipgloss.NewStyle().
		Foreground(textColor).
		MarginBottom(1)

	// Resource styles based on action
	resourceCreateStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(createColor)

	resourceDestroyStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(destroyColor)

	resourceUpdateStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(updateColor)

	resourceReplaceStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(replaceColor)

	resourceReadStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(readColor)

	// Attribute styles
	attrNameStyle = lipgloss.NewStyle().
		Foreground(textColor)

	attrOldValueStyle = lipgloss.NewStyle().
		Foreground(destroyColor).
		Strikethrough(true)

	attrNewValueStyle = lipgloss.NewStyle().
		Foreground(createColor)

	attrComputedStyle = lipgloss.NewStyle().
		Foreground(computedColor).
		Italic(true)

	// Muted style for general muted text
	mutedColor = lipgloss.NewStyle().
		Foreground(mutedColorVal)

	// Action symbols
	createSymbol = lipgloss.NewStyle().Foreground(createColor).Render("+")
	destroySymbol = lipgloss.NewStyle().Foreground(destroyColor).Render("-")
	updateSymbol = lipgloss.NewStyle().Foreground(updateColor).Render("~")
	replaceSymbol = lipgloss.NewStyle().Foreground(replaceColor).Render("±")
	readSymbol = lipgloss.NewStyle().Foreground(readColor).Render("≤")
	forgetSymbol = lipgloss.NewStyle().Foreground(mutedColorVal).Render("⊘")

	// Expand/collapse indicators
	expandedIndicator = lipgloss.NewStyle().Foreground(mutedColorVal).Render("▼")
	collapsedIndicator = lipgloss.NewStyle().Foreground(mutedColorVal).Render("▶")

	// Fast equivalents for the hot per-attribute-row rendering path.
	fastAttrName = newFastStyle(attrNameStyle)
	fastAttrOldValue = newFastStyle(attrOldValueStyle)
	fastAttrNewValue = newFastStyle(attrNewValueStyle)
	fastAttrComputed = newFastStyle(attrComputedStyle)
	fastMuted = newFastStyle(mutedColor)
	fastSensitive = newFastStyle(lipgloss.NewStyle().Foreground(replaceColor).Italic(true))
	fastCreate = newFastStyle(lipgloss.NewStyle().Foreground(createColor))
	fastDestroy = newFastStyle(lipgloss.NewStyle().Foreground(destroyColor))

	// Help style
	helpStyle = lipgloss.NewStyle().
		Foreground(mutedColorVal).
		MarginTop(1)

	// Search style
	searchStyle = lipgloss.NewStyle().
		Foreground(headerColor).
		Bold(true)

	// Match highlight
	matchStyle = lipgloss.NewStyle().
		Background(selectedBg).
		Foreground(createColor).
		Bold(true)
}

// GetActionSymbol returns the appropriate symbol for an action
func GetActionSymbol(action string) string {
	switch action {
	case "create":
		return createSymbol
	case "delete":
		return destroySymbol
	case "update":
		return updateSymbol
	case "replace":
		return replaceSymbol
	case "read":
		return readSymbol
	case "forget":
		return forgetSymbol
	case "output":
		return updateSymbol
	default:
		return updateSymbol
	}
}

// GetResourceStyle returns the appropriate style for a resource action
func GetResourceStyle(action string) lipgloss.Style {
	switch action {
	case "create":
		return resourceCreateStyle
	case "delete":
		return resourceDestroyStyle
	case "update":
		return resourceUpdateStyle
	case "replace":
		return resourceReplaceStyle
	case "read", "forget":
		return resourceReadStyle
	case "output":
		return resourceUpdateStyle
	default:
		return resourceUpdateStyle
	}
}

// GetActionColor returns the color for an action type
func GetActionColor(action string) lipgloss.Color {
	switch action {
	case "create":
		return createColor
	case "delete":
		return destroyColor
	case "update":
		return updateColor
	case "replace":
		return replaceColor
	case "read", "forget":
		return readColor
	case "output":
		return updateColor
	default:
		return updateColor
	}
}

// FormatStatusColored returns a color-styled status string for CLI output
func FormatStatusColored(status string) string {
	if status == "" {
		return ""
	}

	var style lipgloss.Style
	var label string

	switch status {
	case "success":
		label = "[SUCCESS]"
		style = lipgloss.NewStyle().Foreground(createColor)
	case "failed":
		label = "[FAILED]"
		style = lipgloss.NewStyle().Foreground(destroyColor)
	case "cancelled":
		label = "[CANCELLED]"
		style = lipgloss.NewStyle().Foreground(updateColor)
	case "pending":
		label = "[PENDING]"
		style = lipgloss.NewStyle().Foreground(updateColor)
	case "nochanges":
		return ""
	default:
		return ""
	}

	return style.Render(label)
}

// FormatHistoryEntryColored formats a history entry with colored status for CLI output
func FormatHistoryEntryColored(timestamp, command, status, path string) string {
	// Command with color (pad first, then color)
	cmdPadded := fmt.Sprintf("%-7s", command)
	cmdStyle := lipgloss.NewStyle()
	switch command {
	case "apply":
		cmdStyle = cmdStyle.Foreground(createColor)
	case "destroy":
		cmdStyle = cmdStyle.Foreground(destroyColor)
	case "plan":
		cmdStyle = cmdStyle.Foreground(headerColor)
	}
	cmdColored := cmdStyle.Render(cmdPadded)

	// Status with color (pad the label first, then color)
	var statusColored string
	statusPadded := fmt.Sprintf("%-12s", "") // default empty padding
	switch status {
	case "success":
		statusPadded = fmt.Sprintf("%-12s", "[SUCCESS]")
		statusColored = lipgloss.NewStyle().Foreground(createColor).Render(statusPadded)
	case "failed":
		statusPadded = fmt.Sprintf("%-12s", "[FAILED]")
		statusColored = lipgloss.NewStyle().Foreground(destroyColor).Render(statusPadded)
	case "cancelled":
		statusPadded = fmt.Sprintf("%-12s", "[CANCELLED]")
		statusColored = lipgloss.NewStyle().Foreground(updateColor).Render(statusPadded)
	case "pending":
		statusPadded = fmt.Sprintf("%-12s", "[PENDING]")
		statusColored = lipgloss.NewStyle().Foreground(updateColor).Render(statusPadded)
	default:
		statusColored = statusPadded // no color, just spaces
	}

	// Pad path to 40 chars for consistent line width
	pathPadded := fmt.Sprintf("%-40s", path)

	return fmt.Sprintf("%s  %s  %s  %s",
		timestamp,
		cmdColored,
		statusColored,
		pathPadded,
	)
}
