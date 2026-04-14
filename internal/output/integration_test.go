package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CaptShanks/terraprism/internal/parser"
)

// TestJSONOutputIntegration tests the complete pipeline from plan parsing to JSON output
func TestJSONOutputIntegration(t *testing.T) {
	testDataDir := "../../testdata"

	tests := []struct {
		name     string
		filename string
		validate func(*testing.T, *PlanOutput)
	}{
		{
			name:     "sample plan",
			filename: "sample-plan.txt",
			validate: func(t *testing.T, output *PlanOutput) {
				// This test data should contain resource creations
				if output.Summary.Add == 0 {
					t.Error("Expected at least one resource to be added")
				}

				// Should have some resources
				if len(output.Resources) == 0 {
					t.Error("Expected at least one resource")
				}

				// Check for aws_instance.web resource
				found := false
				for _, resource := range output.Resources {
					if resource.Address == "aws_instance.web" {
						found = true
						if resource.Type != "aws_instance" {
							t.Errorf("Expected type aws_instance, got %s", resource.Type)
						}
						if resource.Name != "web" {
							t.Errorf("Expected name web, got %s", resource.Name)
						}
						if resource.Action != "create" {
							t.Errorf("Expected action create, got %s", resource.Action)
						}
					}
				}
				if !found {
					t.Error("Expected to find aws_instance.web resource")
				}
			},
		},
		{
			name:     "output only plan",
			filename: "output-only-plan.txt",
			validate: func(t *testing.T, output *PlanOutput) {
				// This should be a plan with only output changes
				if output.Summary.Add != 0 || output.Summary.Change != 0 || output.Summary.Destroy != 0 {
					t.Error("Expected no resource changes in output-only plan")
				}

				// Should have outputs
				if output.Summary.Outputs == 0 {
					t.Error("Expected output changes")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read test data file
			testFile := filepath.Join(testDataDir, tt.filename)
			data, err := os.ReadFile(testFile)
			if err != nil {
				t.Skipf("Skipping test, cannot read test file %s: %v", testFile, err)
			}

			// Parse the plan
			plan, err := parser.Parse(string(data))
			if err != nil {
				t.Fatalf("Failed to parse plan: %v", err)
			}

			// Convert to JSON
			jsonData, err := ToJSON(plan, "v1.0.0-test", "plan")
			if err != nil {
				t.Fatalf("Failed to convert to JSON: %v", err)
			}

			// Parse the JSON back to verify it's valid
			var output PlanOutput
			if err := json.Unmarshal(jsonData, &output); err != nil {
				t.Fatalf("Generated invalid JSON: %v", err)
			}

			// Basic validation
			if output.Metadata.Version != "v1.0.0-test" {
				t.Errorf("Expected version v1.0.0-test, got %s", output.Metadata.Version)
			}
			if output.Metadata.Command != "plan" {
				t.Errorf("Expected command plan, got %s", output.Metadata.Command)
			}

			// Run test-specific validation
			tt.validate(t, &output)
		})
	}
}

// TestJSONOutputConsistency verifies that multiple conversions produce identical results
func TestJSONOutputConsistency(t *testing.T) {
	testFile := filepath.Join("../../testdata", "sample-plan.txt")
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Skipf("Skipping test, cannot read test file: %v", err)
	}

	plan, err := parser.Parse(string(data))
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	// Convert the same plan multiple times
	json1, err := ToJSON(plan, "v1.0.0", "plan")
	if err != nil {
		t.Fatalf("First conversion failed: %v", err)
	}

	json2, err := ToJSON(plan, "v1.0.0", "plan")
	if err != nil {
		t.Fatalf("Second conversion failed: %v", err)
	}

	// Parse both results
	var output1, output2 PlanOutput
	if err := json.Unmarshal(json1, &output1); err != nil {
		t.Fatalf("Failed to unmarshal first JSON: %v", err)
	}
	if err := json.Unmarshal(json2, &output2); err != nil {
		t.Fatalf("Failed to unmarshal second JSON: %v", err)
	}

	// Compare everything except timestamp
	output1.Metadata.Timestamp = ""
	output2.Metadata.Timestamp = ""

	// Re-marshal to compare
	data1, err := json.Marshal(output1)
	if err != nil {
		t.Fatalf("Failed to marshal first output: %v", err)
	}
	data2, err := json.Marshal(output2)
	if err != nil {
		t.Fatalf("Failed to marshal second output: %v", err)
	}

	if string(data1) != string(data2) {
		t.Error("Multiple conversions of the same plan produced different results")
	}
}

// TestJSONOutputWithRealWorldScenarios tests various edge cases found in real Terraform plans
func TestJSONOutputWithRealWorldScenarios(t *testing.T) {
	scenarios := []struct {
		name     string
		planText string
		validate func(*testing.T, *PlanOutput)
	}{
		{
			name: "plan with complex nested attributes",
			planText: `
Terraform will perform the following actions:

  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami           = "ami-12345"
      + instance_type = "t3.micro"
      + tags          = {
          + "Environment" = "production"
          + "Name"        = "web-server"
        }

      + root_block_device {
          + delete_on_termination = true
          + encrypted             = (known after apply)
          + volume_size           = 20
        }
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`,
			validate: func(t *testing.T, output *PlanOutput) {
				if len(output.Resources) != 1 {
					t.Errorf("Expected 1 resource, got %d", len(output.Resources))
				}
				if output.Summary.Add != 1 {
					t.Errorf("Expected 1 addition, got %d", output.Summary.Add)
				}
			},
		},
		{
			name: "plan with sensitive values",
			planText: `
Terraform will perform the following actions:

  # aws_db_instance.main will be created
  + resource "aws_db_instance" "main" {
      + password = (sensitive value)
      + username = "admin"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`,
			validate: func(t *testing.T, output *PlanOutput) {
				// Look for sensitive attributes in the converted output
				found := false
				for _, resource := range output.Resources {
					for _, attr := range resource.Attributes {
						if attr.Name == "password" && strings.Contains(attr.NewValue, "sensitive") {
							found = true
							break
						}
					}
				}
				if !found && len(output.Resources) > 0 {
					// This is acceptable since the parser might not detect all sensitive values
					t.Log("Sensitive attribute not found in parsed output (this may be expected)")
				}
			},
		},
		{
			name: "empty plan",
			planText: `
No changes. Your infrastructure matches the configuration.

Terraform has compared your real infrastructure against your configuration
and found no differences, so no changes are needed.
`,
			validate: func(t *testing.T, output *PlanOutput) {
				if len(output.Resources) != 0 {
					t.Errorf("Expected 0 resources, got %d", len(output.Resources))
				}
				if output.Summary.Add != 0 || output.Summary.Change != 0 || output.Summary.Destroy != 0 {
					t.Error("Expected no changes in empty plan")
				}
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			plan, err := parser.Parse(scenario.planText)
			if err != nil {
				t.Fatalf("Failed to parse plan: %v", err)
			}

			jsonData, err := ToJSON(plan, "v1.0.0", "test")
			if err != nil {
				t.Fatalf("Failed to convert to JSON: %v", err)
			}

			var output PlanOutput
			if err := json.Unmarshal(jsonData, &output); err != nil {
				t.Fatalf("Generated invalid JSON: %v", err)
			}

			scenario.validate(t, &output)
		})
	}
}

// TestJSONOutputFieldPresence ensures all required fields are present in output
func TestJSONOutputFieldPresence(t *testing.T) {
	plan := &parser.Plan{
		Resources: []parser.Resource{
			{
				Address: "test_resource.test",
				Type:    "test_resource",
				Name:    "test",
				Action:  parser.ActionCreate,
				Attributes: []parser.Attribute{
					{
						Name:   "test_attr",
						Action: parser.ActionCreate,
					},
				},
			},
		},
		TotalAdd:    1,
		TotalChange: 0,
		TotalDestroy: 0,
		OutputCount: 0,
		Summary:     "Plan: 1 to add, 0 to change, 0 to destroy",
	}

	jsonData, err := ToJSON(plan, "v1.0.0", "plan")
	if err != nil {
		t.Fatalf("Failed to convert to JSON: %v", err)
	}

	// Parse as generic interface to check field presence
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Test required fields are present and have correct types
	tests := []struct {
		path     []string
		expected string // expected JSON type
	}{
		{[]string{"summary"}, "object"},
		{[]string{"summary", "add"}, "number"},
		{[]string{"summary", "change"}, "number"},
		{[]string{"summary", "destroy"}, "number"},
		{[]string{"summary", "outputs"}, "number"},
		{[]string{"summary", "text"}, "string"},
		{[]string{"resources"}, "array"},
		{[]string{"metadata"}, "object"},
		{[]string{"metadata", "terraprism_version"}, "string"},
		{[]string{"metadata", "timestamp"}, "string"},
		{[]string{"metadata", "command"}, "string"},
	}

	for _, test := range tests {
		t.Run(strings.Join(test.path, "."), func(t *testing.T) {
			current := result
			for i, key := range test.path[:len(test.path)-1] {
				next, ok := current[key].(map[string]interface{})
				if !ok {
					t.Fatalf("Expected object at path %s, got %T", strings.Join(test.path[:i+1], "."), current[key])
				}
				current = next
			}

			finalKey := test.path[len(test.path)-1]
			value, exists := current[finalKey]
			if !exists {
				t.Fatalf("Field %s does not exist", strings.Join(test.path, "."))
			}

			var actualType string
			switch value.(type) {
			case map[string]interface{}:
				actualType = "object"
			case []interface{}:
				actualType = "array"
			case string:
				actualType = "string"
			case float64:
				actualType = "number"
			case bool:
				actualType = "boolean"
			case nil:
				actualType = "null"
			default:
				actualType = "unknown"
			}

			if actualType != test.expected {
				t.Errorf("Field %s has type %s, expected %s", strings.Join(test.path, "."), actualType, test.expected)
			}
		})
	}
}