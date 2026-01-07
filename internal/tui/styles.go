package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme colors - Soft, low-contrast palette inspired by Tokyo Night / Catppuccin
var (
	// Primary colors for actions (muted, pastel tones)
	createColor  = lipgloss.Color("#9ece6a") // Soft sage green
	destroyColor = lipgloss.Color("#f7768e") // Soft coral red
	updateColor  = lipgloss.Color("#e0af68") // Warm amber
	replaceColor = lipgloss.Color("#bb9af7") // Soft lavender
	readColor    = lipgloss.Color("#7dcfff") // Soft sky blue

	// UI colors
	selectedBg    = lipgloss.Color("#292e42") // Deep navy selection
	headerColor   = lipgloss.Color("#7aa2f7") // Soft periwinkle
	borderColor   = lipgloss.Color("#3b4261") // Muted slate
	mutedColorVal = lipgloss.Color("#565f89") // Soft gray-blue
	textColor     = lipgloss.Color("#a9b1d6") // Soft lavender gray
	computedColor = lipgloss.Color("#73daca") // Soft teal
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
		Background(lipgloss.Color("#3b4261")).
		Foreground(lipgloss.Color("#9ece6a")).
		Bold(true)

	// Border style for sections
	sectionBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	// Status bar
	statusBarStyle = lipgloss.NewStyle().
		Foreground(mutedColorVal).
		Background(lipgloss.Color("#1a1b26")).
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

