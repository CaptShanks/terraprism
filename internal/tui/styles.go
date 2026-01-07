package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme colors - Nightfox color palette
// https://github.com/EdenEast/nightfox.nvim
var (
	// Primary colors for actions
	createColor  = lipgloss.Color("#81b29a") // Nightfox green
	destroyColor = lipgloss.Color("#c94f6d") // Nightfox red
	updateColor  = lipgloss.Color("#dbc074") // Nightfox yellow
	replaceColor = lipgloss.Color("#9d79d6") // Nightfox magenta
	readColor    = lipgloss.Color("#63cdcf") // Nightfox cyan

	// UI colors
	selectedBg    = lipgloss.Color("#2b3b51") // Nightfox selection
	headerColor   = lipgloss.Color("#719cd6") // Nightfox blue
	borderColor   = lipgloss.Color("#39506d") // Nightfox border
	mutedColorVal = lipgloss.Color("#738091") // Nightfox comment
	textColor     = lipgloss.Color("#cdcecf") // Nightfox foreground
	computedColor = lipgloss.Color("#63cdcf") // Nightfox cyan
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
		Background(lipgloss.Color("#2b3b51")).
		Foreground(lipgloss.Color("#81b29a")).
		Bold(true)

	// Border style for sections
	sectionBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
		Foreground(mutedColorVal).
		Background(lipgloss.Color("#192330")).
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

