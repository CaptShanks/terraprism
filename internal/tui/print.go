package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/tfplanview/tfplanview/internal/parser"
)

func init() {
	// Force color output even when not a TTY (for piping)
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// PrintPlan outputs the plan with colors to stdout (non-interactive mode)
func PrintPlan(plan *parser.Plan) {
	// Header
	fmt.Println(headerStyle.Render("📋 tfplanview - Terraform Plan Viewer"))
	fmt.Println()

	// Summary
	if plan.Summary != "" {
		summary := fmt.Sprintf("Plan: %s to add, %s to change, %s to destroy",
			lipgloss.NewStyle().Foreground(createColor).Bold(true).Render(fmt.Sprintf("%d", plan.TotalAdd)),
			lipgloss.NewStyle().Foreground(updateColor).Bold(true).Render(fmt.Sprintf("%d", plan.TotalChange)),
			lipgloss.NewStyle().Foreground(destroyColor).Bold(true).Render(fmt.Sprintf("%d", plan.TotalDestroy)),
		)
		fmt.Println(summary)
	} else {
		fmt.Printf("%d resources with changes\n", len(plan.Resources))
	}
	fmt.Println()

	// Resources
	for _, r := range plan.Resources {
		printResource(r)
		fmt.Println()
	}
}

func printResource(r parser.Resource) {
	// Action symbol and resource address
	symbol := GetActionSymbol(string(r.Action))
	style := GetResourceStyle(string(r.Action))
	
	actionDesc := getActionDesc(r.Action)
	
	fmt.Printf("%s %s %s\n", 
		symbol,
		style.Render(r.Address),
		mutedColor.Render(actionDesc),
	)

	// Attributes
	for _, attr := range r.Attributes {
		printAttribute(attr, r.Action)
	}

	// Raw lines if no parsed attributes
	if len(r.Attributes) == 0 && len(r.RawLines) > 1 {
		for _, raw := range r.RawLines[1:] {
			trimmed := strings.TrimSpace(raw)
			if trimmed != "" && trimmed != "{" && trimmed != "}" {
				fmt.Printf("    %s\n", mutedColor.Render(raw))
			}
		}
	}
}

func printAttribute(attr parser.Attribute, parentAction parser.Action) {
	var symbol string
	switch attr.Action {
	case parser.ActionCreate:
		symbol = lipgloss.NewStyle().Foreground(createColor).Render("+")
	case parser.ActionDestroy:
		symbol = lipgloss.NewStyle().Foreground(destroyColor).Render("-")
	case parser.ActionUpdate:
		symbol = lipgloss.NewStyle().Foreground(updateColor).Render("~")
	default:
		symbol = " "
	}

	nameStyle := lipgloss.NewStyle().Foreground(textColor)
	
	var valueStr string
	if attr.Computed {
		valueStr = lipgloss.NewStyle().Foreground(computedColor).Italic(true).Render("(known after apply)")
	} else if attr.OldValue != "" && attr.NewValue != "" {
		oldStyle := lipgloss.NewStyle().Foreground(destroyColor)
		newStyle := lipgloss.NewStyle().Foreground(createColor)
		valueStr = fmt.Sprintf("%s → %s",
			oldStyle.Render(truncate(attr.OldValue, 40)),
			newStyle.Render(truncate(attr.NewValue, 40)),
		)
	} else if attr.NewValue != "" {
		var style lipgloss.Style
		if attr.Action == parser.ActionDestroy || parentAction == parser.ActionDestroy {
			style = lipgloss.NewStyle().Foreground(destroyColor)
		} else {
			style = lipgloss.NewStyle().Foreground(createColor)
		}
		valueStr = style.Render(truncate(attr.NewValue, 60))
	} else if attr.OldValue != "" {
		valueStr = lipgloss.NewStyle().Foreground(destroyColor).Render(truncate(attr.OldValue, 60))
	}

	if valueStr != "" {
		fmt.Printf("    %s %s = %s\n", symbol, nameStyle.Render(attr.Name), valueStr)
	} else {
		fmt.Printf("    %s %s\n", symbol, nameStyle.Render(attr.Name))
	}
}

func getActionDesc(action parser.Action) string {
	switch action {
	case parser.ActionCreate:
		return "will be created"
	case parser.ActionDestroy:
		return "will be destroyed"
	case parser.ActionUpdate:
		return "will be updated"
	case parser.ActionReplace:
		return "must be replaced"
	case parser.ActionRead:
		return "will be read"
	case parser.ActionDeleteCreate:
		return "will be destroyed then created"
	case parser.ActionCreateDelete:
		return "will be created then destroyed"
	default:
		return ""
	}
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	// Remove surrounding quotes
	if len(s) > 1 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

