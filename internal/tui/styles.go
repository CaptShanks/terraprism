package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Color palette variables - will be set based on background detection
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

// Catppuccin Mocha (Dark) palette
var darkPalette = map[string]string{
	"green":    "#a6e3a1",
	"red":      "#f38ba8",
	"yellow":   "#f9e2af",
	"mauve":    "#cba6f7",
	"sapphire": "#74c7ec",
	"blue":     "#89b4fa",
	"teal":     "#94e2d5",
	"text":     "#cdd6f4",
	"subtext":  "#a6adc8",
	"overlay":  "#7f849c",
	"surface1": "#45475a",
	"surface0": "#313244",
	"mantle":   "#181825",
	"base":     "#1e1e2e",
}

// Catppuccin Latte (Light) palette
var lightPalette = map[string]string{
	"green":    "#40a02b",
	"red":      "#d20f39",
	"yellow":   "#df8e1d",
	"mauve":    "#8839ef",
	"sapphire": "#209fb5",
	"blue":     "#1e66f5",
	"teal":     "#179299",
	"text":     "#4c4f69",
	"subtext":  "#6c6f85",
	"overlay":  "#8c8fa1",
	"surface1": "#bcc0cc",
	"surface0": "#ccd0da",
	"mantle":   "#e6e9ef",
	"base":     "#eff1f5",
}

// IsLightBackground returns true if terminal has a light background
func IsLightBackground() bool {
	return !termenv.HasDarkBackground()
}

func init() {
	// Initialize colors based on detected background
	InitColors()
}

// InitColors sets the color palette based on terminal background
func InitColors() {
	if IsLightBackground() {
		SetLightPalette()
	} else {
		SetDarkPalette()
	}
	initStyles()
}

// SetDarkPalette sets colors for dark backgrounds (Catppuccin Mocha)
func SetDarkPalette() {
	createColor = lipgloss.Color(darkPalette["green"])
	destroyColor = lipgloss.Color(darkPalette["red"])
	updateColor = lipgloss.Color(darkPalette["yellow"])
	replaceColor = lipgloss.Color(darkPalette["mauve"])
	readColor = lipgloss.Color(darkPalette["sapphire"])
	selectedBg = lipgloss.Color(darkPalette["surface1"])
	headerColor = lipgloss.Color(darkPalette["blue"])
	mutedColorVal = lipgloss.Color(darkPalette["overlay"])
	textColor = lipgloss.Color(darkPalette["text"])
	computedColor = lipgloss.Color(darkPalette["teal"])
	initStyles() // Reinitialize styles with new colors
}

// SetLightPalette sets colors for light backgrounds (Catppuccin Latte)
func SetLightPalette() {
	createColor = lipgloss.Color(lightPalette["green"])
	destroyColor = lipgloss.Color(lightPalette["red"])
	updateColor = lipgloss.Color(lightPalette["yellow"])
	replaceColor = lipgloss.Color(lightPalette["mauve"])
	readColor = lipgloss.Color(lightPalette["sapphire"])
	selectedBg = lipgloss.Color(lightPalette["surface1"])
	headerColor = lipgloss.Color(lightPalette["blue"])
	mutedColorVal = lipgloss.Color(lightPalette["overlay"])
	textColor = lipgloss.Color(lightPalette["text"])
	computedColor = lipgloss.Color(lightPalette["teal"])
	initStyles() // Reinitialize styles with new colors
}

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
	expandedIndicator  string
	collapsedIndicator string
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

	// Expand/collapse indicators
	expandedIndicator = lipgloss.NewStyle().Foreground(mutedColorVal).Render("▼")
	collapsedIndicator = lipgloss.NewStyle().Foreground(mutedColorVal).Render("▶")

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
	case "destroy":
		return destroySymbol
	case "update":
		return updateSymbol
	case "replace", "delete-create", "create-delete":
		return replaceSymbol
	case "read":
		return readSymbol
	default:
		return updateSymbol
	}
}

// GetResourceStyle returns the appropriate style for a resource action
func GetResourceStyle(action string) lipgloss.Style {
	switch action {
	case "create":
		return resourceCreateStyle
	case "destroy":
		return resourceDestroyStyle
	case "update":
		return resourceUpdateStyle
	case "replace", "delete-create", "create-delete":
		return resourceReplaceStyle
	case "read":
		return resourceReadStyle
	default:
		return resourceUpdateStyle
	}
}

// GetActionColor returns the color for an action type
func GetActionColor(action string) lipgloss.Color {
	switch action {
	case "create":
		return createColor
	case "destroy":
		return destroyColor
	case "update":
		return updateColor
	case "replace", "delete-create", "create-delete":
		return replaceColor
	case "read":
		return readColor
	default:
		return updateColor
	}
}
