package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme colors - Catppuccin Mocha (Crisp)
// Brighter, more vivid variants for better visibility
var (
	// Primary colors for actions (crisp, vivid tones)
	createColor  = lipgloss.Color("#40d97f") // vivid green
	destroyColor = lipgloss.Color("#ff6b8a") // vivid red
	updateColor  = lipgloss.Color("#ffd866") // vivid yellow
	replaceColor = lipgloss.Color("#d4a5ff") // vivid mauve
	readColor    = lipgloss.Color("#5ccfe6") // vivid sapphire

	// UI colors
	selectedBg    = lipgloss.Color("#45475a") // surface1
	headerColor   = lipgloss.Color("#7dcfff") // vivid blue
	borderColor   = lipgloss.Color("#313244") // surface0
	mutedColorVal = lipgloss.Color("#9399b2") // overlay2 (brighter)
	textColor     = lipgloss.Color("#e4e8f7") // brighter text
	computedColor = lipgloss.Color("#5de4c7") // vivid teal
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
		Foreground(lipgloss.Color("#40d97f")). // vivid green
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

