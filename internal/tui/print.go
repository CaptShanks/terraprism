package tui

import (
	"fmt"
	"strings"

	"github.com/CaptShanks/terraprism/internal/tfplan"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Force color output even when not a TTY (for piping)
	lipgloss.SetColorProfile(termenv.TrueColor)
}

// PrintPlan outputs the plan with colors to stdout (non-interactive mode)
func PrintPlan(plan *tfplan.Plan) {
	// Header
	fmt.Println(headerStyle.Render("🔺 Terra-Prism - Terraform Plan Viewer"))
	fmt.Println()

	// Summary
	summary := fmt.Sprintf("Plan: %s to add, %s to change, %s to destroy",
		lipgloss.NewStyle().Foreground(createColor).Bold(true).Render(fmt.Sprintf("%d", plan.TotalAdd)),
		lipgloss.NewStyle().Foreground(updateColor).Bold(true).Render(fmt.Sprintf("%d", plan.TotalChange)),
		lipgloss.NewStyle().Foreground(destroyColor).Bold(true).Render(fmt.Sprintf("%d", plan.TotalDestroy)),
	)
	if plan.OutputCount > 0 {
		summary += fmt.Sprintf(", %s output(s) changed",
			lipgloss.NewStyle().Foreground(updateColor).Bold(true).Render(fmt.Sprintf("%d", plan.OutputCount)),
		)
	}
	fmt.Println(summary)
	fmt.Println()

	// Resources (including output changes, appended as synthetic entries)
	for _, r := range plan.DisplayResources() {
		printResource(r)
		fmt.Println()
	}
}

func printResource(r tfplan.Resource) {
	symbol := GetActionSymbol(string(r.Action))
	style := GetResourceStyle(string(r.Action))
	actionDesc := getActionDescription(r)

	fmt.Printf("%s %s %s\n",
		symbol,
		style.Render(r.Address),
		fastMuted.Render(actionDesc),
	)

	printAttributeTree(r.Attributes, 1, true)
}

// printAttributeTree walks a resource's (or container's) attribute tree,
// printing one row per leaf/fold-header with depth-based indent — the
// flat-mode equivalent of Model.renderAttributeTree, minus fold/cursor
// state since print mode always shows everything expanded.
func printAttributeTree(attrs []tfplan.Attribute, depth int, keyed bool) {
	indent := strings.Repeat("  ", depth)
	for i := 0; i < len(attrs); i++ {
		attr := attrs[i]

		if attr.Action == tfplan.ActionNoOp {
			run := 1
			for i+run < len(attrs) && attrs[i+run].Action == tfplan.ActionNoOp {
				run++
			}
			fmt.Println(indent + fastMuted.Render(unchangedHiddenNote(run)))
			i += run - 1
			continue
		}

		if isContainerAttr(attr) {
			open, closeBracket := containerBrackets(attr.Kind)
			var content string
			if keyed {
				content = fastAttrName.Render(attr.Name) + " = " + fastMuted.Render(open)
			} else {
				content = fastMuted.Render(open)
			}
			fmt.Println(indent + actionPrefixSymbol(attr.Action) + " " + content)
			printAttributeTree(attr.Children, depth+1, attr.Kind == tfplan.KindMap)
			fmt.Println(indent + fastMuted.Render(closeBracket))
			continue
		}

		if attr.Sensitive {
			fmt.Println(indent + actionPrefixSymbol(attr.Action) + " " + renderKeyValue(attr, keyed))
			continue
		}

		if isMultilineStringAttr(attr) {
			fmt.Println(printMultilineStringDiff(attr, indent, keyed))
			continue
		}

		fmt.Println(indent + actionPrefixSymbol(attr.Action) + " " + renderKeyValue(attr, keyed))
	}
}

// printMultilineStringDiff is print.go's flat-mode equivalent of
// Model.renderMultilineStringDiff (no diff-context clamping, since print
// mode has no interactive context-size control — always shows the full
// line-level diff).
func printMultilineStringDiff(attr tfplan.Attribute, indent string, keyed bool) string {
	var b strings.Builder
	if keyed {
		b.WriteString(indent + actionPrefixSymbol(attr.Action) + " " + fastAttrName.Render(attr.Name) + " = " + fastMuted.Render("<<EOT") + "\n")
	} else {
		b.WriteString(indent + actionPrefixSymbol(attr.Action) + " " + fastMuted.Render("<<EOT") + "\n")
	}
	contentIndent := indent + "  "

	oldStr, _ := attr.Old.(string)
	newStr, _ := attr.New.(string)

	switch attr.Action {
	case tfplan.ActionCreate:
		for _, l := range strings.Split(newStr, "\n") {
			b.WriteString(contentIndent + fastCreate.Render("+ "+l) + "\n")
		}
	case tfplan.ActionDelete:
		for _, l := range strings.Split(oldStr, "\n") {
			b.WriteString(contentIndent + fastDestroy.Render("- "+l) + "\n")
		}
	case tfplan.ActionNoOp:
		for _, l := range strings.Split(newStr, "\n") {
			b.WriteString(contentIndent + fastMuted.Render("  "+l) + "\n")
		}
	default:
		diff := ComputeDiff(strings.Split(oldStr, "\n"), strings.Split(newStr, "\n"))
		for _, d := range diff {
			switch d.Op {
			case DiffDelete:
				b.WriteString(contentIndent + fastDestroy.Render("- "+d.Text) + "\n")
			case DiffInsert:
				b.WriteString(contentIndent + fastCreate.Render("+ "+d.Text) + "\n")
			case DiffEqual:
				b.WriteString(contentIndent + fastMuted.Render("  "+d.Text) + "\n")
			}
		}
	}
	b.WriteString(indent + fastMuted.Render("EOT"))
	return b.String()
}
