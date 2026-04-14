package output

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/CaptShanks/terraprism/internal/parser"
)

func TestConvertToJSON(t *testing.T) {
	tests := []struct {
		name    string
		plan    *parser.Plan
		version string
		command string
		want    func(*PlanOutput) bool
	}{
		{
			name: "empty plan",
			plan: &parser.Plan{
				Resources:    []parser.Resource{},
				Summary:      "Plan: 0 to add, 0 to change, 0 to destroy",
				TotalAdd:     0,
				TotalChange:  0,
				TotalDestroy: 0,
				OutputCount:  0,
			},
			version: "v1.0.0",
			command: "plan",
			want: func(output *PlanOutput) bool {
				return output.Summary.Add == 0 &&
					output.Summary.Change == 0 &&
					output.Summary.Destroy == 0 &&
					output.Summary.Outputs == 0 &&
					len(output.Resources) == 0 &&
					output.Metadata.Version == "v1.0.0" &&
					output.Metadata.Command == "plan"
			},
		},
		{
			name: "plan with single resource creation",
			plan: &parser.Plan{
				Resources: []parser.Resource{
					{
						Address: "aws_instance.web",
						Type:    "aws_instance",
						Name:    "web",
						Action:  parser.ActionCreate,
						Attributes: []parser.Attribute{
							{
								Name:     "ami",
								NewValue: "ami-0c55b159cbfafe1f0",
								Action:   parser.ActionCreate,
							},
							{
								Name:     "instance_type",
								NewValue: "t3.micro",
								Action:   parser.ActionCreate,
							},
						},
					},
				},
				Summary:      "Plan: 1 to add, 0 to change, 0 to destroy",
				TotalAdd:     1,
				TotalChange:  0,
				TotalDestroy: 0,
				OutputCount:  0,
			},
			version: "v1.0.0",
			command: "plan",
			want: func(output *PlanOutput) bool {
				return output.Summary.Add == 1 &&
					len(output.Resources) == 1 &&
					output.Resources[0].Address == "aws_instance.web" &&
					output.Resources[0].Type == "aws_instance" &&
					output.Resources[0].Name == "web" &&
					output.Resources[0].Action == "create" &&
					len(output.Resources[0].Attributes) == 2
			},
		},
		{
			name: "plan with resource update",
			plan: &parser.Plan{
				Resources: []parser.Resource{
					{
						Address: "aws_instance.web",
						Type:    "aws_instance",
						Name:    "web",
						Action:  parser.ActionUpdate,
						Attributes: []parser.Attribute{
							{
								Name:     "tags",
								OldValue: `{"Name" = "old-name"}`,
								NewValue: `{"Name" = "new-name"}`,
								Action:   parser.ActionUpdate,
							},
							{
								Name:      "id",
								NewValue:  "i-1234567890abcdef0",
								Action:    parser.ActionCreate,
								Computed:  true,
								Sensitive: false,
							},
						},
					},
				},
				Summary:      "Plan: 0 to add, 1 to change, 0 to destroy",
				TotalAdd:     0,
				TotalChange:  1,
				TotalDestroy: 0,
				OutputCount:  0,
			},
			version: "v1.0.0",
			command: "plan",
			want: func(output *PlanOutput) bool {
				if output.Summary.Change != 1 || len(output.Resources) != 1 {
					return false
				}
				resource := output.Resources[0]
				if resource.Action != "update" || len(resource.Attributes) != 2 {
					return false
				}
				// Check tags attribute
				tagsAttr := resource.Attributes[0]
				if tagsAttr.Name != "tags" || tagsAttr.Action != "update" ||
					tagsAttr.OldValue != `{"Name" = "old-name"}` ||
					tagsAttr.NewValue != `{"Name" = "new-name"}` {
					return false
				}
				// Check computed attribute
				idAttr := resource.Attributes[1]
				return idAttr.Computed == true && idAttr.Sensitive == false
			},
		},
		{
			name: "plan with sensitive attribute",
			plan: &parser.Plan{
				Resources: []parser.Resource{
					{
						Address: "aws_db_instance.main",
						Type:    "aws_db_instance",
						Name:    "main",
						Action:  parser.ActionCreate,
						Attributes: []parser.Attribute{
							{
								Name:      "password",
								NewValue:  "********",
								Action:    parser.ActionCreate,
								Sensitive: true,
							},
						},
					},
				},
				Summary:      "Plan: 1 to add, 0 to change, 0 to destroy",
				TotalAdd:     1,
				TotalChange:  0,
				TotalDestroy: 0,
				OutputCount:  0,
			},
			version: "v1.0.0",
			command: "plan",
			want: func(output *PlanOutput) bool {
				return len(output.Resources) == 1 &&
					len(output.Resources[0].Attributes) == 1 &&
					output.Resources[0].Attributes[0].Sensitive == true &&
					output.Resources[0].Attributes[0].Name == "password"
			},
		},
		{
			name: "plan with multiple actions",
			plan: &parser.Plan{
				Resources: []parser.Resource{
					{
						Address: "aws_instance.web1",
						Type:    "aws_instance",
						Name:    "web1",
						Action:  parser.ActionCreate,
					},
					{
						Address: "aws_instance.web2",
						Type:    "aws_instance",
						Name:    "web2",
						Action:  parser.ActionDestroy,
					},
					{
						Address: "aws_instance.web3",
						Type:    "aws_instance",
						Name:    "web3",
						Action:  parser.ActionReplace,
					},
				},
				Summary:      "Plan: 1 to add, 0 to change, 2 to destroy",
				TotalAdd:     1,
				TotalChange:  0,
				TotalDestroy: 2,
				OutputCount:  0,
			},
			version: "v1.0.0",
			command: "plan",
			want: func(output *PlanOutput) bool {
				if len(output.Resources) != 3 {
					return false
				}
				actions := make(map[string]bool)
				for _, resource := range output.Resources {
					actions[resource.Action] = true
				}
				return actions["create"] && actions["destroy"] && actions["replace"]
			},
		},
		{
			name: "plan with outputs",
			plan: &parser.Plan{
				Resources: []parser.Resource{
					{
						Address: "Changes to Outputs",
						Type:    "output",
						Name:    "outputs",
						Action:  parser.ActionOutput,
					},
				},
				Summary:      "Plan: 0 to add, 0 to change, 0 to destroy",
				TotalAdd:     0,
				TotalChange:  0,
				TotalDestroy: 0,
				OutputCount:  3,
			},
			version: "v1.0.0",
			command: "plan",
			want: func(output *PlanOutput) bool {
				return output.Summary.Outputs == 3 &&
					len(output.Resources) == 1 &&
					output.Resources[0].Type == "output" &&
					output.Resources[0].Action == "output"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			output, err := ConvertToJSON(tt.plan, tt.version, tt.command)
			if err != nil {
				t.Fatalf("ConvertToJSON() error = %v", err)
			}

			if !tt.want(output) {
				t.Errorf("ConvertToJSON() validation failed for %s", tt.name)
			}

			// Verify metadata timestamp is valid
			if _, err := time.Parse(time.RFC3339, output.Metadata.Timestamp); err != nil {
				t.Errorf("ConvertToJSON() invalid timestamp format: %v", err)
			}
		})
	}
}

func TestMarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		output  *PlanOutput
		wantErr bool
	}{
		{
			name: "valid output",
			output: &PlanOutput{
				Summary: SummaryOutput{
					Add:     1,
					Change:  0,
					Destroy: 0,
					Outputs: 0,
					Text:    "Plan: 1 to add, 0 to change, 0 to destroy",
				},
				Resources: []ResourceOutput{
					{
						Address: "aws_instance.web",
						Type:    "aws_instance",
						Name:    "web",
						Action:  "create",
						Attributes: []AttributeOutput{
							{
								Name:     "ami",
								NewValue: "ami-12345",
								Action:   "create",
							},
						},
					},
				},
				Metadata: MetadataOutput{
					Version:   "v1.0.0",
					Timestamp: time.Now().UTC().Format(time.RFC3339),
					Command:   "plan",
				},
			},
			wantErr: false,
		},
		{
			name: "empty output",
			output: &PlanOutput{
				Summary:   SummaryOutput{},
				Resources: []ResourceOutput{},
				Metadata:  MetadataOutput{},
			},
			wantErr: false,
		},
		{
			name: "nil resources slice",
			output: &PlanOutput{
				Summary:   SummaryOutput{},
				Resources: nil,
				Metadata:  MetadataOutput{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := MarshalJSON(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("MarshalJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the JSON is valid by unmarshaling it
				var unmarshaled map[string]interface{}
				if err := json.Unmarshal(data, &unmarshaled); err != nil {
					t.Errorf("MarshalJSON() produced invalid JSON: %v", err)
				}

				// Verify the JSON is pretty-printed (indented)
				if len(data) > 0 && !containsIndentation(string(data)) {
					t.Error("MarshalJSON() should produce indented JSON")
				}
			}
		})
	}
}

func TestToJSON(t *testing.T) {
	tests := []struct {
		name    string
		plan    *parser.Plan
		version string
		command string
		wantErr bool
	}{
		{
			name: "successful conversion",
			plan: &parser.Plan{
				Resources: []parser.Resource{
					{
						Address: "aws_instance.web",
						Type:    "aws_instance",
						Name:    "web",
						Action:  parser.ActionCreate,
					},
				},
				Summary:     "Plan: 1 to add, 0 to change, 0 to destroy",
				TotalAdd:    1,
				TotalChange: 0,
				TotalDestroy: 0,
				OutputCount: 0,
			},
			version: "v1.0.0",
			command: "plan",
			wantErr: false,
		},
		{
			name: "nil plan",
			plan: &parser.Plan{
				Resources:    nil,
				TotalAdd:     0,
				TotalChange:  0,
				TotalDestroy: 0,
				OutputCount:  0,
			},
			version: "v1.0.0",
			command: "plan",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := ToJSON(tt.plan, tt.version, tt.command)
			if (err != nil) != tt.wantErr {
				t.Errorf("ToJSON() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify the result is valid JSON
				var unmarshaled PlanOutput
				if err := json.Unmarshal(data, &unmarshaled); err != nil {
					t.Errorf("ToJSON() produced invalid JSON: %v", err)
				}

				// Verify basic structure
				if unmarshaled.Metadata.Version != tt.version {
					t.Errorf("ToJSON() version = %v, want %v", unmarshaled.Metadata.Version, tt.version)
				}
				if unmarshaled.Metadata.Command != tt.command {
					t.Errorf("ToJSON() command = %v, want %v", unmarshaled.Metadata.Command, tt.command)
				}
			}
		})
	}
}

func TestJSONSchemaStability(t *testing.T) {
	// This test ensures that the JSON schema remains stable over time
	plan := &parser.Plan{
		Resources: []parser.Resource{
			{
				Address: "aws_instance.web",
				Type:    "aws_instance",
				Name:    "web",
				Action:  parser.ActionCreate,
				Attributes: []parser.Attribute{
					{
						Name:      "ami",
						NewValue:  "ami-12345",
						Action:    parser.ActionCreate,
						Computed:  false,
						Sensitive: false,
					},
					{
						Name:      "password",
						NewValue:  "********",
						Action:    parser.ActionCreate,
						Sensitive: true,
					},
				},
			},
		},
		Summary:      "Plan: 1 to add, 0 to change, 0 to destroy",
		TotalAdd:     1,
		TotalChange:  0,
		TotalDestroy: 0,
		OutputCount:  0,
	}

	data, err := ToJSON(plan, "v1.0.0", "plan")
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Verify required top-level fields exist
	requiredFields := []string{"summary", "resources", "metadata"}
	for _, field := range requiredFields {
		if _, exists := result[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Verify summary structure
	summary, ok := result["summary"].(map[string]interface{})
	if !ok {
		t.Fatal("Summary should be an object")
	}
	summaryFields := []string{"add", "change", "destroy", "outputs", "text"}
	for _, field := range summaryFields {
		if _, exists := summary[field]; !exists {
			t.Errorf("Missing summary field: %s", field)
		}
	}

	// Verify resources structure
	resources, ok := result["resources"].([]interface{})
	if !ok {
		t.Fatal("Resources should be an array")
	}
	if len(resources) > 0 {
		resource := resources[0].(map[string]interface{})
		resourceFields := []string{"address", "type", "name", "action", "attributes"}
		for _, field := range resourceFields {
			if _, exists := resource[field]; !exists {
				t.Errorf("Missing resource field: %s", field)
			}
		}

		// Verify attributes structure
		attributes, ok := resource["attributes"].([]interface{})
		if !ok {
			t.Fatal("Attributes should be an array")
		}
		if len(attributes) > 0 {
			attr := attributes[0].(map[string]interface{})
			attrFields := []string{"name", "action"}
			for _, field := range attrFields {
				if _, exists := attr[field]; !exists {
					t.Errorf("Missing attribute field: %s", field)
				}
			}
		}
	}

	// Verify metadata structure
	metadata, ok := result["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("Metadata should be an object")
	}
	metadataFields := []string{"terraprism_version", "timestamp", "command"}
	for _, field := range metadataFields {
		if _, exists := metadata[field]; !exists {
			t.Errorf("Missing metadata field: %s", field)
		}
	}
}

func TestAttributeOutputEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		attr parser.Attribute
		want AttributeOutput
	}{
		{
			name: "empty values",
			attr: parser.Attribute{
				Name:     "empty_attr",
				Action:   parser.ActionCreate,
				Computed: false,
				Sensitive: false,
			},
			want: AttributeOutput{
				Name:      "empty_attr",
				OldValue:  "",
				NewValue:  "",
				Action:    "create",
				Computed:  false,
				Sensitive: false,
			},
		},
		{
			name: "computed and sensitive",
			attr: parser.Attribute{
				Name:      "computed_sensitive",
				NewValue:  "(known after apply)",
				Action:    parser.ActionCreate,
				Computed:  true,
				Sensitive: true,
			},
			want: AttributeOutput{
				Name:      "computed_sensitive",
				OldValue:  "",
				NewValue:  "(known after apply)",
				Action:    "create",
				Computed:  true,
				Sensitive: true,
			},
		},
		{
			name: "update with old and new values",
			attr: parser.Attribute{
				Name:     "update_attr",
				OldValue: "old_value",
				NewValue: "new_value",
				Action:   parser.ActionUpdate,
			},
			want: AttributeOutput{
				Name:     "update_attr",
				OldValue: "old_value",
				NewValue: "new_value",
				Action:   "update",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &parser.Plan{
				Resources: []parser.Resource{
					{
						Address:    "test_resource.test",
						Type:       "test_resource",
						Name:       "test",
						Action:     parser.ActionCreate,
						Attributes: []parser.Attribute{tt.attr},
					},
				},
			}

			output, err := ConvertToJSON(plan, "v1.0.0", "plan")
			if err != nil {
				t.Fatalf("ConvertToJSON() error = %v", err)
			}

			if len(output.Resources) != 1 || len(output.Resources[0].Attributes) != 1 {
				t.Fatal("Expected 1 resource with 1 attribute")
			}

			got := output.Resources[0].Attributes[0]
			if got.Name != tt.want.Name {
				t.Errorf("Name = %v, want %v", got.Name, tt.want.Name)
			}
			if got.OldValue != tt.want.OldValue {
				t.Errorf("OldValue = %v, want %v", got.OldValue, tt.want.OldValue)
			}
			if got.NewValue != tt.want.NewValue {
				t.Errorf("NewValue = %v, want %v", got.NewValue, tt.want.NewValue)
			}
			if got.Action != tt.want.Action {
				t.Errorf("Action = %v, want %v", got.Action, tt.want.Action)
			}
			if got.Computed != tt.want.Computed {
				t.Errorf("Computed = %v, want %v", got.Computed, tt.want.Computed)
			}
			if got.Sensitive != tt.want.Sensitive {
				t.Errorf("Sensitive = %v, want %v", got.Sensitive, tt.want.Sensitive)
			}
		})
	}
}

func TestResourceOutputEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		resource parser.Resource
		validate func(ResourceOutput) bool
	}{
		{
			name: "resource with no attributes",
			resource: parser.Resource{
				Address:    "aws_instance.empty",
				Type:       "aws_instance",
				Name:       "empty",
				Action:     parser.ActionDestroy,
				Attributes: []parser.Attribute{},
			},
			validate: func(ro ResourceOutput) bool {
				return ro.Address == "aws_instance.empty" &&
					ro.Action == "destroy" &&
					len(ro.Attributes) == 0 &&
					ro.Attributes != nil // should be empty slice, not nil
			},
		},
		{
			name: "resource with many attributes",
			resource: parser.Resource{
				Address: "aws_instance.complex",
				Type:    "aws_instance",
				Name:    "complex",
				Action:  parser.ActionUpdate,
				Attributes: func() []parser.Attribute {
					attrs := make([]parser.Attribute, 100)
					for i := 0; i < 100; i++ {
						attrs[i] = parser.Attribute{
							Name:   fmt.Sprintf("attr_%d", i),
							Action: parser.ActionUpdate,
						}
					}
					return attrs
				}(),
			},
			validate: func(ro ResourceOutput) bool {
				return len(ro.Attributes) == 100 &&
					ro.Attributes[0].Name == "attr_0" &&
					ro.Attributes[99].Name == "attr_99"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan := &parser.Plan{
				Resources: []parser.Resource{tt.resource},
			}

			output, err := ConvertToJSON(plan, "v1.0.0", "plan")
			if err != nil {
				t.Fatalf("ConvertToJSON() error = %v", err)
			}

			if len(output.Resources) != 1 {
				t.Fatal("Expected 1 resource")
			}

			if !tt.validate(output.Resources[0]) {
				t.Error("Resource validation failed")
			}
		})
	}
}

// Helper function to check if JSON contains indentation
func containsIndentation(jsonStr string) bool {
	return len(jsonStr) > 10 && (jsonStr[1] == '\n' || jsonStr[2] == ' ')
}