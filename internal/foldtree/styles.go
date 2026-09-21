package foldtree

import "github.com/charmbracelet/lipgloss"

// Minimal default styling shared by foldtree's own widgets (Picker,
// LogPane). Callers rendering tree rows themselves (via RowRenderer) are
// free to ignore these entirely and use their own palette — these exist
// only for the chrome foldtree itself draws.
var (
	selectedRowStyle    = lipgloss.NewStyle().Background(lipgloss.Color("240"))
	mutedTextStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	matchHighlightStyle = lipgloss.NewStyle().Background(lipgloss.Color("220")).Foreground(lipgloss.Color("0"))
)
