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

		// Check for resource header
		if match := resourceRegex.FindStringSubmatch(line); match != nil {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}

			address := strings.TrimSpace(match[1])
			action := parseActionFromLine(line)

			currentResource = &Resource{
				Address:  address,
				Action:   action,
				RawLines: []string{line},
			}

			// Extract type and name from address
			parts := strings.Split(address, ".")
			if len(parts) >= 2 {
				currentResource.Type = parts[len(parts)-2]
				currentResource.Name = parts[len(parts)-1]
			}

			inResourceBlock = true
			braceCount = 0
			continue
		}

		if inResourceBlock && currentResource != nil {
			currentResource.RawLines = append(currentResource.RawLines, line)

			// Count braces to track block depth
			braceCount += strings.Count(line, "{") - strings.Count(line, "}")

			// Parse attributes
			if match := attrRegex.FindStringSubmatch(line); match != nil {
				symbol := match[1]
				name := strings.TrimSpace(match[2])
				value := strings.TrimSpace(match[3])

				attr := Attribute{
					Name: name,
				}

				switch symbol {
				case "+":
					attr.Action = ActionCreate
					attr.NewValue = value
				case "-":
					attr.Action = ActionDestroy
					attr.OldValue = value
				case "~":
					attr.Action = ActionUpdate
					// Try to parse old -> new value
					if strings.Contains(value, " -> ") {
						parts := strings.SplitN(value, " -> ", 2)
						attr.OldValue = strings.TrimSpace(parts[0])
						attr.NewValue = strings.TrimSpace(parts[1])
					} else {
						attr.NewValue = value
					}
				}

				// Check for computed or sensitive markers
				if strings.Contains(value, "(known after apply)") {
					attr.Computed = true
				}
				if strings.Contains(value, "(sensitive") {
					attr.Sensitive = true
				}

				currentResource.Attributes = append(currentResource.Attributes, attr)
			} else if match := attrRegex2.FindStringSubmatch(line); match != nil {
				// Simpler attribute format
				symbol := match[1]
				content := strings.TrimSpace(match[2])

				attr := Attribute{
					Name: content,
				}

				switch symbol {
				case "+":
					attr.Action = ActionCreate
				case "-":
					attr.Action = ActionDestroy
				case "~":
					attr.Action = ActionUpdate
				}

				currentResource.Attributes = append(currentResource.Attributes, attr)
			}

			// Check if resource block ended
			if braceCount <= 0 && strings.TrimSpace(line) == "}" {
				inResourceBlock = false
			}
		}
	}

	// Don't forget the last resource
	if currentResource != nil {
		plan.Resources = append(plan.Resources, *currentResource)
	}
}

// parseOldFormat parses Terraform 0.11 and earlier format plans
func parseOldFormat(plan *Plan, lines []string) {
	// Old format patterns
	createRegex := regexp.MustCompile(`^\+\s+(.+)$`)
	destroyRegex := regexp.MustCompile(`^-\s+(.+)$`)
	updateRegex := regexp.MustCompile(`^~\s+(.+)$`)
	replaceRegex := regexp.MustCompile(`^-/\+\s+(.+)$`)
	replaceRegex2 := regexp.MustCompile(`^\+/-\s+(.+)$`)
	attrRegex := regexp.MustCompile(`^\s+([^:]+):\s*(.*)$`)

	var currentResource *Resource

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Check for resource lines
		if match := createRegex.FindStringSubmatch(line); match != nil && !strings.Contains(line, ":") {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}
			address := strings.TrimSpace(match[1])
			currentResource = &Resource{
				Address:  address,
				Action:   ActionCreate,
				RawLines: []string{line},
			}
			parseResourceAddress(currentResource)
			continue
		}

		if match := destroyRegex.FindStringSubmatch(line); match != nil && !strings.Contains(line, ":") {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}
			address := strings.TrimSpace(match[1])
			currentResource = &Resource{
				Address:  address,
				Action:   ActionDestroy,
				RawLines: []string{line},
			}
			parseResourceAddress(currentResource)
			continue
		}

		if match := updateRegex.FindStringSubmatch(line); match != nil && !strings.Contains(line, ":") {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}
			address := strings.TrimSpace(match[1])
			currentResource = &Resource{
				Address:  address,
				Action:   ActionUpdate,
				RawLines: []string{line},
			}
			parseResourceAddress(currentResource)
			continue
		}

		if match := replaceRegex.FindStringSubmatch(line); match != nil {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}
			address := strings.TrimSpace(match[1])
			currentResource = &Resource{
				Address:  address,
				Action:   ActionReplace,
				RawLines: []string{line},
			}
			parseResourceAddress(currentResource)
			continue
		}

		if match := replaceRegex2.FindStringSubmatch(line); match != nil {
			if currentResource != nil {
				plan.Resources = append(plan.Resources, *currentResource)
			}
			address := strings.TrimSpace(match[1])
			currentResource = &Resource{
				Address:  address,
				Action:   ActionCreateDelete,
				RawLines: []string{line},
			}
			parseResourceAddress(currentResource)
			continue
		}

		// Parse attributes for current resource
		if currentResource != nil {
			currentResource.RawLines = append(currentResource.RawLines, line)

			if match := attrRegex.FindStringSubmatch(line); match != nil {
				name := strings.TrimSpace(match[1])
				value := strings.TrimSpace(match[2])

				attr := Attribute{
					Name: name,
				}

				// Check for change indicator
				if strings.Contains(value, " => ") {
					parts := strings.SplitN(value, " => ", 2)
					attr.OldValue = strings.TrimSpace(parts[0])
					attr.NewValue = strings.TrimSpace(parts[1])
					attr.Action = ActionUpdate
				} else {
					attr.NewValue = value
					attr.Action = currentResource.Action
				}

				if strings.Contains(value, "<computed>") {
					attr.Computed = true
				}

				currentResource.Attributes = append(currentResource.Attributes, attr)
			}
		}
	}

	// Don't forget the last resource
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
