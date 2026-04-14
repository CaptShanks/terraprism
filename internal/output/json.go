package output

import (
	"encoding/json"
	"time"

	"github.com/CaptShanks/terraprism/internal/parser"
)

// PlanOutput represents the complete JSON output structure
type PlanOutput struct {
	Summary   SummaryOutput    `json:"summary"`
	Resources []ResourceOutput `json:"resources"`
	Metadata  MetadataOutput   `json:"metadata"`
}

// SummaryOutput contains aggregate statistics
type SummaryOutput struct {
	Add     int    `json:"add"`
	Change  int    `json:"change"`
	Destroy int    `json:"destroy"`
	Outputs int    `json:"outputs"`
	Text    string `json:"text"`
}

// ResourceOutput represents a single resource change
type ResourceOutput struct {
	Address    string            `json:"address"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Action     string            `json:"action"`
	Attributes []AttributeOutput `json:"attributes"`
}

// AttributeOutput represents an attribute change
type AttributeOutput struct {
	Name      string `json:"name"`
	OldValue  string `json:"old_value,omitempty"`
	NewValue  string `json:"new_value,omitempty"`
	Action    string `json:"action"`
	Computed  bool   `json:"computed,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

// MetadataOutput contains execution metadata
type MetadataOutput struct {
	Version   string `json:"terraprism_version"`
	Timestamp string `json:"timestamp"`
	Command   string `json:"command"`
}

// ConvertToJSON converts a parser.Plan to JSON output format
func ConvertToJSON(plan *parser.Plan, version string, command string) (*PlanOutput, error) {
	output := &PlanOutput{
		Summary: SummaryOutput{
			Add:     plan.TotalAdd,
			Change:  plan.TotalChange,
			Destroy: plan.TotalDestroy,
			Outputs: plan.OutputCount,
			Text:    plan.Summary,
		},
		Resources: make([]ResourceOutput, 0, len(plan.Resources)),
		Metadata: MetadataOutput{
			Version:   version,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Command:   command,
		},
	}

	for _, resource := range plan.Resources {
		resourceOutput := ResourceOutput{
			Address:    resource.Address,
			Type:       resource.Type,
			Name:       resource.Name,
			Action:     string(resource.Action),
			Attributes: make([]AttributeOutput, 0, len(resource.Attributes)),
		}

		for _, attr := range resource.Attributes {
			attributeOutput := AttributeOutput{
				Name:      attr.Name,
				OldValue:  attr.OldValue,
				NewValue:  attr.NewValue,
				Action:    string(attr.Action),
				Computed:  attr.Computed,
				Sensitive: attr.Sensitive,
			}
			resourceOutput.Attributes = append(resourceOutput.Attributes, attributeOutput)
		}

		output.Resources = append(output.Resources, resourceOutput)
	}

	return output, nil
}

// MarshalJSON serializes a PlanOutput to JSON bytes
func MarshalJSON(output *PlanOutput) ([]byte, error) {
	return json.MarshalIndent(output, "", "  ")
}

// ToJSON converts a parser.Plan directly to JSON bytes
func ToJSON(plan *parser.Plan, version string, command string) ([]byte, error) {
	output, err := ConvertToJSON(plan, version, command)
	if err != nil {
		return nil, err
	}
	return MarshalJSON(output)
}