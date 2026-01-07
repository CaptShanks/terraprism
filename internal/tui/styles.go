package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme colors - Catppuccin Mocha palette
// Matching user's starship terminal colors
var (
	// Primary colors for actions
	createColor  = lipgloss.Color("#a6e3a1") // green
	destroyColor = lipgloss.Color("#f38ba8") // red
	updateColor  = lipgloss.Color("#f9e2af") // yellow
	replaceColor = lipgloss.Color("#cba6f7") // mauve
	readColor    = lipgloss.Color("#74c7ec") // sapphire

	// UI colors
	selectedBg    = lipgloss.Color("#45475a") // surface1
	headerColor   = lipgloss.Color("#89b4fa") // blue
	borderColor   = lipgloss.Color("#313244") // surface0
	mutedColorVal = lipgloss.Color("#7f849c") // overlay1
	textColor     = lipgloss.Color("#cdd6f4") // text
	computedColor = lipgloss.Color("#94e2d5") // teal
)

// Styles
var (
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

	// Selected row
	selectedStyle = lipgloss.NewStyle().
		Background(selectedBg)

	// Attribute styles
	attrNameStyle = lipgloss.NewStyle().
		Foreground(textColor)

	attrValueStyle = lipgloss.NewStyle().
		Foreground(mutedColorVal)

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
	createSymbol  = lipgloss.NewStyle().Foreground(createColor).Render("+")
	destroySymbol = lipgloss.NewStyle().Foreground(destroyColor).Render("-")
	updateSymbol  = lipgloss.NewStyle().Foreground(updateColor).Render("~")
	replaceSymbol = lipgloss.NewStyle().Foreground(replaceColor).Render("±")
	readSymbol    = lipgloss.NewStyle().Foreground(readColor).Render("≤")

	// Expand/collapse indicators
	expandedIndicator  = lipgloss.NewStyle().Foreground(mutedColorVal).Render("▼")
	collapsedIndicator = lipgloss.NewStyle().Foreground(mutedColorVal).Render("▶")

	// Help style
	helpStyle = lipgloss.NewStyle().
		Foreground(mutedColorVal).
		MarginTop(1)

	// Search style
	searchStyle = lipgloss.NewStyle().
		Foreground(headerColor).
		Bold(true)

	searchInputStyle = lipgloss.NewStyle().
		Foreground(textColor).
		Background(selectedBg).
		Padding(0, 1)

	// Match highlight
	matchStyle = lipgloss.NewStyle().
		Background(lipgloss.Color("#45475a")). // surface1
		Foreground(lipgloss.Color("#a6e3a1")). // green
		Bold(true)

	// Border style for sections
	sectionBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
		Foreground(mutedColorVal).
		Background(lipgloss.Color("#181825")). // mantle
		Padding(0, 1)
)

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

