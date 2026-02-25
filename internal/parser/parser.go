package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// Action represents the type of change being made to a resource
type Action string

const (
	ActionCreate       Action = "create"
	ActionDestroy      Action = "destroy"
	ActionUpdate       Action = "update"
	ActionReplace      Action = "replace"
	ActionRead         Action = "read"
	ActionNoOp         Action = "no-op"
	ActionCreateDelete Action = "create-delete"
	ActionDeleteCreate Action = "delete-create"
)

// Attribute represents a single attribute change
type Attribute struct {
	Name      string
	OldValue  string
	NewValue  string
	Action    Action
	Computed  bool
	Sensitive bool
}

// Resource represents a single resource in the plan
type Resource struct {
	Address    string
	Type       string
	Name       string
	Action     Action
	Attributes []Attribute
	RawLines   []string
}

// Plan represents a parsed Terraform plan
type Plan struct {
	Resources    []Resource
	Summary      string
	TotalAdd     int
	TotalChange  int
	TotalDestroy int
	RawPlan      string
}

// Parse parses a Terraform plan output string
func Parse(input string) (*Plan, error) {
	plan := &Plan{
		RawPlan: input,
	}

	lines := strings.Split(input, "\n")

	// Try to detect format and parse accordingly
	if isNewFormat(lines) {
		parseNewFormat(plan, lines)
	} else {
		parseOldFormat(plan, lines)
	}

	// Parse summary
	parseSummary(plan, lines)

	return plan, nil
}

// isNewFormat checks if the plan is in Terraform 0.12+ format
func isNewFormat(lines []string) bool {
	for _, line := range lines {
		// New format uses # for resource headers
		if strings.Contains(line, "# ") && (strings.Contains(line, " will be ") ||
			strings.Contains(line, " must be ") ||
			strings.Contains(line, " has been ") ||
			strings.Contains(line, " is tainted")) {
			return true
		}
	}
	return false
}

func parseNewFormatAttrFromMatch(symbol, name, value string) *Attribute {
	attr := &Attribute{Name: name}
	switch symbol {
	case "+":
		attr.Action = ActionCreate
		attr.NewValue = value
	case "-":
		attr.Action = ActionDestroy
		attr.OldValue = value
	case "~":
		attr.Action = ActionUpdate
		if strings.Contains(value, " -> ") {
			parts := strings.SplitN(value, " -> ", 2)
			attr.OldValue = strings.TrimSpace(parts[0])
			attr.NewValue = strings.TrimSpace(parts[1])
		} else {
			attr.NewValue = value
		}
	}
	if strings.Contains(value, "(known after apply)") {
		attr.Computed = true
	}
	if strings.Contains(value, "(sensitive") {
		attr.Sensitive = true
	}
	return attr
}

func parseNewFormatAttrSimple(symbol, content string) *Attribute {
	attr := &Attribute{Name: content}
	switch symbol {
	case "+":
		attr.Action = ActionCreate
	case "-":
		attr.Action = ActionDestroy
	case "~":
		attr.Action = ActionUpdate
	}
	return attr
}

// parseNewFormat parses Terraform 0.12+ format plans
func parseNewFormat(plan *Plan, lines []string) {
	resourceRegex := regexp.MustCompile(`^\s*#\s+(.+?)\s+(will be|must be|has been|is tainted)`)
	attrRegex := regexp.MustCompile(`^\s+([~+\-])\s+"?([^"=]+)"?\s*=\s*(.*)`)
	attrRegex2 := regexp.MustCompile(`^\s+([~+\-])\s+(.+)$`)

	var currentResource *Resource
	inResourceBlock := false
	braceCount := 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if match := resourceRegex.FindStringSubmatch(line); match != nil {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}
			address := strings.TrimSpace(match[1])
			currentResource = &Resource{
				Address:  address,
				Action:  parseActionFromLine(line),
				RawLines: []string{line},
			}
			if parts := strings.Split(address, "."); len(parts) >= 2 {
				currentResource.Type = parts[len(parts)-2]
				currentResource.Name = parts[len(parts)-1]
			}
			inResourceBlock = true
			braceCount = 0
			continue
		}

		if inResourceBlock && currentResource != nil {
			currentResource.RawLines = append(currentResource.RawLines, line)
			braceCount += strings.Count(line, "{") - strings.Count(line, "}")

			if match := attrRegex.FindStringSubmatch(line); match != nil {
				attr := parseNewFormatAttrFromMatch(match[1], strings.TrimSpace(match[2]), strings.TrimSpace(match[3]))
				currentResource.Attributes = append(currentResource.Attributes, *attr)
			} else if match := attrRegex2.FindStringSubmatch(line); match != nil {
				attr := parseNewFormatAttrSimple(match[1], strings.TrimSpace(match[2]))
				currentResource.Attributes = append(currentResource.Attributes, *attr)
			}

			if braceCount <= 0 && strings.TrimSpace(line) == "}" {
				inResourceBlock = false
			}
		}
	}
	if currentResource != nil {
		plan.Resources = append(plan.Resources, *currentResource)
	}
}

type oldFormatResourcePattern struct {
	re       *regexp.Regexp
	action   Action
	noColon  bool // resource lines (not attribute lines) must not contain ":"
}

var oldFormatPatterns = []oldFormatResourcePattern{
	{regexp.MustCompile(`^\+\s+(.+)$`), ActionCreate, true},
	{regexp.MustCompile(`^-\s+(.+)$`), ActionDestroy, true},
	{regexp.MustCompile(`^~\s+(.+)$`), ActionUpdate, true},
	{regexp.MustCompile(`^-/\+\s+(.+)$`), ActionReplace, false},
	{regexp.MustCompile(`^\+/-\s+(.+)$`), ActionCreateDelete, false},
}

func parseOldFormatResourceLine(line string) (address string, action Action, ok bool) {
	for _, p := range oldFormatPatterns {
		if p.noColon && strings.Contains(line, ":") {
			continue
		}
		if match := p.re.FindStringSubmatch(line); match != nil {
			return strings.TrimSpace(match[1]), p.action, true
		}
	}
	return "", "", false
}

func parseOldFormatAttrLine(line string, res *Resource) *Attribute {
	attrRegex := regexp.MustCompile(`^\s+([^:]+):\s*(.*)$`)
	match := attrRegex.FindStringSubmatch(line)
	if match == nil {
		return nil
	}
	name := strings.TrimSpace(match[1])
	value := strings.TrimSpace(match[2])
	attr := &Attribute{Name: name}
	if strings.Contains(value, " => ") {
		parts := strings.SplitN(value, " => ", 2)
		attr.OldValue = strings.TrimSpace(parts[0])
		attr.NewValue = strings.TrimSpace(parts[1])
		attr.Action = ActionUpdate
	} else {
		attr.NewValue = value
		attr.Action = res.Action
	}
	if strings.Contains(value, "<computed>") {
		attr.Computed = true
	}
	return attr
}

// parseOldFormat parses Terraform 0.11 and earlier format plans
func parseOldFormat(plan *Plan, lines []string) {
	var currentResource *Resource
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if address, action, ok := parseOldFormatResourceLine(line); ok {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}
			currentResource = &Resource{Address: address, Action: action, RawLines: []string{line}}
			parseResourceAddress(currentResource)
			continue
		}
		if currentResource != nil {
			currentResource.RawLines = append(currentResource.RawLines, line)
			if attr := parseOldFormatAttrLine(line, currentResource); attr != nil {
				currentResource.Attributes = append(currentResource.Attributes, *attr)
			}
		}
	}
	if currentResource != nil {
		plan.Resources = append(plan.Resources, *currentResource)
	}
}

func parseResourceAddress(r *Resource) {
	parts := strings.Split(r.Address, ".")
	if len(parts) >= 2 {
		r.Type = parts[len(parts)-2]
		r.Name = parts[len(parts)-1]
	}
}

func parseActionFromLine(line string) Action {
	lower := strings.ToLower(line)

	if strings.Contains(lower, "will be created") || strings.Contains(lower, "has been created") {
		return ActionCreate
	}
	if strings.Contains(lower, "will be destroyed") || strings.Contains(lower, "must be destroyed") {
		return ActionDestroy
	}
	if strings.Contains(lower, "will be updated") || strings.Contains(lower, "has been changed") {
		return ActionUpdate
	}
	if strings.Contains(lower, "must be replaced") || strings.Contains(lower, "will be replaced") {
		return ActionReplace
	}
	if strings.Contains(lower, "will be read") {
		return ActionRead
	}
	if strings.Contains(lower, "is tainted") {
		return ActionReplace
	}

	// Check for destroy-create or create-destroy
	if strings.Contains(lower, "destroyed and then created") || strings.Contains(lower, "-/+") {
		return ActionDeleteCreate
	}
	if strings.Contains(lower, "created and then destroyed") || strings.Contains(lower, "+/-") {
		return ActionCreateDelete
	}

	return ActionUpdate
}

func parseSummary(plan *Plan, lines []string) {
	summaryRegex := regexp.MustCompile(`Plan:\s*(\d+)\s*to add,\s*(\d+)\s*to change,\s*(\d+)\s*to destroy`)

	for _, line := range lines {
		if match := summaryRegex.FindStringSubmatch(line); match != nil {
			plan.Summary = line
			// Parse numbers (ignore errors, default to 0)
			_, _ = fmt.Sscanf(match[1], "%d", &plan.TotalAdd)
			_, _ = fmt.Sscanf(match[2], "%d", &plan.TotalChange)
			_, _ = fmt.Sscanf(match[3], "%d", &plan.TotalDestroy)
			break
		}
	}
}
