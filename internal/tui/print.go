package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/CaptShanks/terraprism/internal/parser"
)

func init() {
	// Force color output even when not a TTY (for piping)
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// PrintPlan outputs the plan with colors to stdout (non-interactive mode)
func PrintPlan(plan *parser.Plan) {
	// Header
	fmt.Println(headerStyle.Render("🔺 Terra-Prism - Terraform Plan Viewer"))
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
	// Action symbol and resource address (header line)
	symbol := GetActionSymbol(string(r.Action))
	style := GetResourceStyle(string(r.Action))

	actionDesc := getActionDesc(r.Action)

	fmt.Printf("%s %s %s\n",
		symbol,
		style.Render(r.Address),
		mutedColor.Render(actionDesc),
	)

	// Print the full HCL block with syntax highlighting
	if len(r.RawLines) > 1 {
		for _, line := range r.RawLines[1:] {
			coloredLine := colorizeLine(line, r.Action)
			fmt.Println(coloredLine)
		}
	}
}

// colorizeLine applies syntax highlighting to a single line of HCL
func colorizeLine(line string, action parser.Action) string {
	// Detect the line prefix symbol (+, -, ~)
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	// Check for change symbols at start of content
	var prefix string
	var content string

	if strings.HasPrefix(trimmed, "+ ") {
		prefix = lipgloss.NewStyle().Foreground(createColor).Render("+")
		content = trimmed[2:]
	} else if strings.HasPrefix(trimmed, "- ") {
		prefix = lipgloss.NewStyle().Foreground(destroyColor).Render("-")
		content = trimmed[2:]
	} else if strings.HasPrefix(trimmed, "~ ") {
		prefix = lipgloss.NewStyle().Foreground(updateColor).Render("~")
		content = trimmed[2:]
	} else {
		prefix = " "
		content = trimmed
	}

	// Apply syntax highlighting to content
	coloredContent := colorizeHCL(content, action)

	return indent + prefix + " " + coloredContent
}

// colorizeHCL applies HCL syntax highlighting
func colorizeHCL(content string, action parser.Action) string {
	// Empty or structural lines
	if content == "" || content == "{" || content == "}" || content == "]" || content == "[" {
		return mutedColor.Render(content)
	}

	// Check for key = value pattern
	kvPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*(.*)$`)
	if match := kvPattern.FindStringSubmatch(content); match != nil {
		key := match[1]
		value := match[2]
		return colorizeKeyValue(key, value, action)
	}

	// Check for "key" = value pattern (quoted keys)
	quotedKvPattern := regexp.MustCompile(`^"([^"]+)"\s*=\s*(.*)$`)
	if match := quotedKvPattern.FindStringSubmatch(content); match != nil {
		key := match[1]
		value := match[2]
		return lipgloss.NewStyle().Foreground(textColor).Render("\""+key+"\"") + " = " + colorizeValue(value, action)
	}

	// Resource/block declarations
	if strings.HasPrefix(content, "resource ") || strings.HasPrefix(content, "data ") {
		return colorizeBlockDeclaration(content)
	}

	// Nested block headers (e.g., "root_block_device {")
	blockPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*\{`)
	if match := blockPattern.FindStringSubmatch(content); match != nil {
		blockName := match[1]
		return lipgloss.NewStyle().Foreground(headerColor).Render(blockName) + " {"
	}

	// Default: just return with text color
	return lipgloss.NewStyle().Foreground(textColor).Render(content)
}

func colorizeKeyValue(key, value string, action parser.Action) string {
	keyStyle := lipgloss.NewStyle().Foreground(textColor)
	return keyStyle.Render(key) + " = " + colorizeValue(value, action)
}

func colorizeValue(value string, action parser.Action) string {
	value = strings.TrimSpace(value)

	// Check for (known after apply)
	if strings.Contains(value, "(known after apply)") {
		return lipgloss.NewStyle().Foreground(computedColor).Italic(true).Render(value)
	}

	// Check for (sensitive value)
	if strings.Contains(value, "(sensitive") {
		return lipgloss.NewStyle().Foreground(replaceColor).Italic(true).Render(value)
	}

	// Check for change arrow: "old" -> "new" or old -> new
	if strings.Contains(value, " -> ") {
		parts := strings.SplitN(value, " -> ", 2)
		oldVal := strings.TrimSpace(parts[0])
		newVal := strings.TrimSpace(parts[1])

		oldStyle := lipgloss.NewStyle().Foreground(destroyColor)
		newStyle := lipgloss.NewStyle().Foreground(createColor)
		arrowStyle := lipgloss.NewStyle().Foreground(mutedColorVal)

		return oldStyle.Render(oldVal) + arrowStyle.Render(" → ") + newStyle.Render(newVal)
	}

	// Check for null
	if value == "null" {
		return lipgloss.NewStyle().Foreground(destroyColor).Render(value)
	}

	// Check for boolean
	if value == "true" || value == "false" {
		return lipgloss.NewStyle().Foreground(readColor).Render(value)
	}

	// Check for numbers
	numberPattern := regexp.MustCompile(`^-?[0-9]+\.?[0-9]*$`)
	if numberPattern.MatchString(value) {
		return lipgloss.NewStyle().Foreground(updateColor).Render(value)
	}

	// Check for strings (quoted)
	if len(value) > 1 && value[0] == '"' {
		return lipgloss.NewStyle().Foreground(createColor).Render(value)
	}

	// Check for opening brace/bracket (start of nested structure)
	if value == "{" || value == "[" || strings.HasSuffix(value, "{") || strings.HasSuffix(value, "[") {
		return lipgloss.NewStyle().Foreground(mutedColorVal).Render(value)
	}

	// Default based on action
	switch action {
	case parser.ActionCreate:
		return lipgloss.NewStyle().Foreground(createColor).Render(value)
	case parser.ActionDestroy:
		return lipgloss.NewStyle().Foreground(destroyColor).Render(value)
	case parser.ActionUpdate:
		return lipgloss.NewStyle().Foreground(updateColor).Render(value)
	default:
		return lipgloss.NewStyle().Foreground(textColor).Render(value)
	}
}

func colorizeBlockDeclaration(content string) string {
	// Pattern: resource "type" "name" {
	pattern := regexp.MustCompile(`^(resource|data)\s+"([^"]+)"\s+"([^"]+)"\s*\{?`)
	if match := pattern.FindStringSubmatch(content); match != nil {
		keyword := match[1]
		resType := match[2]
		resName := match[3]

		keywordStyle := lipgloss.NewStyle().Foreground(replaceColor).Bold(true)
		typeStyle := lipgloss.NewStyle().Foreground(headerColor)
		nameStyle := lipgloss.NewStyle().Foreground(createColor)

		result := keywordStyle.Render(keyword) + " " +
			typeStyle.Render("\""+resType+"\"") + " " +
			nameStyle.Render("\""+resName+"\"")

		if strings.HasSuffix(content, "{") {
			result += " {"
		}
		return result
	}

	return lipgloss.NewStyle().Foreground(textColor).Render(content)
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
