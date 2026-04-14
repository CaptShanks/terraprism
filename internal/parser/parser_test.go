package parser

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestParseNewFormat(t *testing.T) {
	input := `
Terraform will perform the following actions:

  # aws_instance.example will be created
  + resource "aws_instance" "example" {
      + ami                          = "ami-12345678"
      + arn                          = (known after apply)
      + availability_zone            = (known after apply)
      + instance_type                = "t2.micro"
      + tags                         = {
          + "Name" = "example"
        }
    }

  # aws_security_group.web will be updated in-place
  ~ resource "aws_security_group" "web" {
        id                     = "sg-12345678"
        name                   = "web"
      ~ description            = "Old description" -> "New description"
    }

  # aws_s3_bucket.data will be destroyed
  - resource "aws_s3_bucket" "data" {
      - bucket = "my-data-bucket" -> null
    }

Plan: 1 to add, 1 to change, 1 to destroy.
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	if len(plan.Resources) != 3 {
		t.Errorf("Expected 3 resources, got %d", len(plan.Resources))
	}

	// Check first resource (create)
	if plan.Resources[0].Action != ActionCreate {
		t.Errorf("Expected first resource action to be create, got %s", plan.Resources[0].Action)
	}
	if plan.Resources[0].Address != "aws_instance.example" {
		t.Errorf("Expected address 'aws_instance.example', got '%s'", plan.Resources[0].Address)
	}

	// Check second resource (update)
	if plan.Resources[1].Action != ActionUpdate {
		t.Errorf("Expected second resource action to be update, got %s", plan.Resources[1].Action)
	}

	// Check third resource (destroy)
	if plan.Resources[2].Action != ActionDestroy {
		t.Errorf("Expected third resource action to be destroy, got %s", plan.Resources[2].Action)
	}

	// Check summary
	if plan.TotalAdd != 1 {
		t.Errorf("Expected TotalAdd to be 1, got %d", plan.TotalAdd)
	}
	if plan.TotalChange != 1 {
		t.Errorf("Expected TotalChange to be 1, got %d", plan.TotalChange)
	}
	if plan.TotalDestroy != 1 {
		t.Errorf("Expected TotalDestroy to be 1, got %d", plan.TotalDestroy)
	}
}

func TestParseOldFormat(t *testing.T) {
	input := `
+ aws_instance.example
    ami:           "ami-12345678"
    instance_type: "t2.micro"

~ aws_security_group.web
    description: "Old description" => "New description"

- aws_s3_bucket.data

Plan: 1 to add, 1 to change, 1 to destroy.
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	if len(plan.Resources) != 3 {
		t.Errorf("Expected 3 resources, got %d", len(plan.Resources))
	}

	// Check actions
	if plan.Resources[0].Action != ActionCreate {
		t.Errorf("Expected first resource action to be create, got %s", plan.Resources[0].Action)
	}
	if plan.Resources[1].Action != ActionUpdate {
		t.Errorf("Expected second resource action to be update, got %s", plan.Resources[1].Action)
	}
	if plan.Resources[2].Action != ActionDestroy {
		t.Errorf("Expected third resource action to be destroy, got %s", plan.Resources[2].Action)
	}
}

func TestParseReplace(t *testing.T) {
	input := `
  # aws_instance.replaced must be replaced
  -/+ resource "aws_instance" "replaced" {
      ~ ami           = "ami-old" -> "ami-new" # forces replacement
      + instance_type = "t2.micro"
    }

Plan: 1 to add, 0 to change, 1 to destroy.
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	if len(plan.Resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(plan.Resources))
	}

	if plan.Resources[0].Action != ActionReplace {
		t.Errorf("Expected action to be replace, got %s", plan.Resources[0].Action)
	}
}

func TestParseOutputChanges(t *testing.T) {
	input := `
Terraform will perform the following actions:

  # aws_instance.example will be created
  + resource "aws_instance" "example" {
      + ami           = "ami-12345678"
      + instance_type = "t2.micro"
    }

Plan: 1 to add, 0 to change, 0 to destroy.

Changes to Outputs:
  + instance_id        = (known after apply)
  + instance_public_ip = (known after apply)
  ~ sg_id              = "sg-old" -> (known after apply)
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	if len(plan.Resources) != 2 {
		t.Fatalf("Expected 2 resources (1 real + 1 output), got %d", len(plan.Resources))
	}

	if plan.Resources[0].Action != ActionCreate {
		t.Errorf("Expected first resource action to be create, got %s", plan.Resources[0].Action)
	}

	outputRes := plan.Resources[1]
	if outputRes.Action != ActionOutput {
		t.Errorf("Expected second resource action to be output, got %s", outputRes.Action)
	}
	if outputRes.Address != "Changes to Outputs" {
		t.Errorf("Expected address 'Changes to Outputs', got '%s'", outputRes.Address)
	}
	if plan.OutputCount != 3 {
		t.Errorf("Expected OutputCount 3, got %d", plan.OutputCount)
	}
	// RawLines[0] is the header, [1:] are the output lines
	if len(outputRes.RawLines) != 4 {
		t.Errorf("Expected 4 RawLines (1 header + 3 outputs), got %d", len(outputRes.RawLines))
	}
}

func TestParseOutputOnlyPlan(t *testing.T) {
	input := `
No changes. Your infrastructure matches the configuration.

Changes to Outputs:
  + new_output     = "hello-world"
  ~ changed_output = "old-value" -> "new-value"
  - removed_output = "gone" -> null

You can apply this plan to save these new output values to the Terraform
state, without changing any real infrastructure.
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	if len(plan.Resources) != 1 {
		t.Fatalf("Expected 1 resource (synthetic output), got %d", len(plan.Resources))
	}

	outputRes := plan.Resources[0]
	if outputRes.Action != ActionOutput {
		t.Errorf("Expected action to be output, got %s", outputRes.Action)
	}
	if outputRes.Type != "output" {
		t.Errorf("Expected type 'output', got '%s'", outputRes.Type)
	}
	if plan.OutputCount != 3 {
		t.Errorf("Expected OutputCount 3, got %d", plan.OutputCount)
	}
}

func TestParseNoOutputChanges(t *testing.T) {
	input := `
Terraform will perform the following actions:

  # aws_instance.example will be created
  + resource "aws_instance" "example" {
      + ami           = "ami-12345678"
      + instance_type = "t2.micro"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	if len(plan.Resources) != 1 {
		t.Errorf("Expected 1 resource, got %d", len(plan.Resources))
	}
	if plan.Resources[0].Action != ActionCreate {
		t.Errorf("Expected action to be create, got %s", plan.Resources[0].Action)
	}
	if plan.OutputCount != 0 {
		t.Errorf("Expected OutputCount 0, got %d", plan.OutputCount)
	}
}

func TestParseEmptyPlan(t *testing.T) {
	input := `
No changes. Infrastructure is up-to-date.
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	if len(plan.Resources) != 0 {
		t.Errorf("Expected 0 resources, got %d", len(plan.Resources))
	}
}

// TestJSONSerialization tests that parsed plans can be serialized to JSON correctly
func TestJSONSerialization(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedFields map[string]interface{}
		validateFunc   func(t *testing.T, jsonData map[string]interface{})
	}{
		{
			name: "basic plan with create/update/destroy",
			input: `
Terraform will perform the following actions:

  # aws_instance.example will be created
  + resource "aws_instance" "example" {
      + ami           = "ami-12345678"
      + instance_type = "t2.micro"
      + tags          = {
          + "Name" = "example"
        }
    }

  # aws_security_group.web will be updated in-place
  ~ resource "aws_security_group" "web" {
      ~ description = "Old description" -> "New description"
    }

  # aws_s3_bucket.data will be destroyed
  - resource "aws_s3_bucket" "data" {
      - bucket = "my-data-bucket" -> null
    }

Plan: 1 to add, 1 to change, 1 to destroy.
`,
			expectedFields: map[string]interface{}{
				"total_add":     float64(1),
				"total_change":  float64(1),
				"total_destroy": float64(1),
			},
			validateFunc: func(t *testing.T, jsonData map[string]interface{}) {
				resources, ok := jsonData["resources"].([]interface{})
				if !ok || len(resources) != 3 {
					t.Errorf("Expected 3 resources in JSON, got %d", len(resources))
					return
				}

				// Validate first resource (create)
				res0 := resources[0].(map[string]interface{})
				if res0["action"] != "create" {
					t.Errorf("Expected first resource action to be 'create', got %s", res0["action"])
				}
				if res0["address"] != "aws_instance.example" {
					t.Errorf("Expected first resource address to be 'aws_instance.example', got %s", res0["address"])
				}

				// Validate attributes structure
				attrs, ok := res0["attributes"].([]interface{})
				if !ok {
					t.Error("Expected attributes to be an array")
				} else if len(attrs) > 0 {
					attr0 := attrs[0].(map[string]interface{})
					if _, hasName := attr0["name"]; !hasName {
						t.Error("Expected attribute to have 'name' field")
					}
					if _, hasAction := attr0["action"]; !hasAction {
						t.Error("Expected attribute to have 'action' field")
					}
				}
			},
		},
		{
			name: "empty plan",
			input: `
No changes. Infrastructure is up-to-date.
`,
			expectedFields: map[string]interface{}{
				"total_add":     float64(0),
				"total_change":  float64(0),
				"total_destroy": float64(0),
				"output_count":  float64(0),
			},
			validateFunc: func(t *testing.T, jsonData map[string]interface{}) {
				resources, ok := jsonData["resources"].([]interface{})
				if !ok {
					t.Error("Expected 'resources' field to be an array")
					return
				}
				if len(resources) != 0 {
					t.Errorf("Expected 0 resources in JSON for empty plan, got %d", len(resources))
				}
			},
		},
		{
			name: "plan with outputs only",
			input: `
No changes. Your infrastructure matches the configuration.

Changes to Outputs:
  + new_output     = "hello-world"
  ~ changed_output = "old-value" -> "new-value"
  - removed_output = "gone" -> null

You can apply this plan to save these new output values to the Terraform
state, without changing any real infrastructure.
`,
			expectedFields: map[string]interface{}{
				"total_add":     float64(0),
				"total_change":  float64(0),
				"total_destroy": float64(0),
				"output_count":  float64(3),
			},
			validateFunc: func(t *testing.T, jsonData map[string]interface{}) {
				resources, ok := jsonData["resources"].([]interface{})
				if !ok || len(resources) != 1 {
					t.Errorf("Expected 1 synthetic output resource in JSON, got %d", len(resources))
					return
				}

				outputRes := resources[0].(map[string]interface{})
				if outputRes["action"] != "output" {
					t.Errorf("Expected output resource action to be 'output', got %s", outputRes["action"])
				}
				if outputRes["address"] != "Changes to Outputs" {
					t.Errorf("Expected output resource address to be 'Changes to Outputs', got %s", outputRes["address"])
				}
			},
		},
		{
			name: "plan with sensitive and computed attributes",
			input: `
Terraform will perform the following actions:

  # aws_db_instance.main will be created
  + resource "aws_db_instance" "main" {
      + password      = (sensitive value)
      + endpoint      = (known after apply)
      + username      = "admin"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`,
			validateFunc: func(t *testing.T, jsonData map[string]interface{}) {
				resources, ok := jsonData["resources"].([]interface{})
				if !ok || len(resources) != 1 {
					t.Errorf("Expected 1 resource in JSON, got %d", len(resources))
					return
				}

				res := resources[0].(map[string]interface{})
				attrs, ok := res["attributes"].([]interface{})
				if !ok {
					t.Error("Expected attributes to be an array")
					return
				}

				// Look for sensitive and computed flags
				foundSensitive := false
				foundComputed := false
				for _, attr := range attrs {
					attrMap := attr.(map[string]interface{})
					if attrMap["name"] == "password" && attrMap["sensitive"] == true {
						foundSensitive = true
					}
					if attrMap["name"] == "endpoint" && attrMap["computed"] == true {
						foundComputed = true
					}
				}

				if !foundSensitive {
					t.Error("Expected to find sensitive attribute for password")
				}
				if !foundComputed {
					t.Error("Expected to find computed attribute for endpoint")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			plan, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Failed to parse plan: %v", err)
			}

			// Test JSON marshaling
			jsonBytes, err := json.MarshalIndent(plan, "", "  ")
			if err != nil {
				t.Fatalf("Failed to marshal plan to JSON: %v", err)
			}

			// Validate it's valid JSON
			var jsonData map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &jsonData); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Check expected fields
			for field, expectedValue := range tt.expectedFields {
				if actual, exists := jsonData[field]; !exists {
					t.Errorf("Expected field '%s' to exist in JSON output", field)
				} else if actual != expectedValue {
					t.Errorf("Expected field '%s' to be %v, got %v", field, expectedValue, actual)
				}
			}

			// Check that raw_plan is included
			if rawPlan, exists := jsonData["raw_plan"]; !exists {
				t.Error("Expected 'raw_plan' field to exist in JSON output")
			} else if rawPlanStr, ok := rawPlan.(string); !ok {
				t.Error("Expected 'raw_plan' to be a string")
			} else if !strings.Contains(rawPlanStr, "Terraform") && !strings.Contains(rawPlanStr, "No changes") {
				t.Error("Expected 'raw_plan' to contain original plan text")
			}

			// Run custom validation if provided
			if tt.validateFunc != nil {
				tt.validateFunc(t, jsonData)
			}
		})
	}
}

// TestJSONSerializationWithSampleData tests JSON serialization using testdata files
func TestJSONSerializationWithSampleData(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{
			name:     "comprehensive sample plan",
			filename: "../../testdata/sample-plan.txt",
		},
		{
			name:     "output only plan",
			filename: "../../testdata/output-only-plan.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(tt.filename)
			if err != nil {
				t.Skipf("Skipping test, could not read file %s: %v", tt.filename, err)
			}

			plan, err := Parse(string(data))
			if err != nil {
				t.Fatalf("Failed to parse plan from %s: %v", tt.filename, err)
			}

			// Test JSON marshaling
			jsonBytes, err := json.MarshalIndent(plan, "", "  ")
			if err != nil {
				t.Fatalf("Failed to marshal plan to JSON: %v", err)
			}

			// Validate it's valid JSON and can be unmarshaled
			var jsonData map[string]interface{}
			if err := json.Unmarshal(jsonBytes, &jsonData); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}

			// Validate schema structure
			requiredFields := []string{"resources", "summary", "total_add", "total_change", "total_destroy", "output_count", "raw_plan"}
			for _, field := range requiredFields {
				if _, exists := jsonData[field]; !exists {
					t.Errorf("Expected required field '%s' to exist in JSON output", field)
				}
			}

			// Validate resources array structure
			if resources, ok := jsonData["resources"].([]interface{}); ok {
				for i, resource := range resources {
					resMap, ok := resource.(map[string]interface{})
					if !ok {
						t.Errorf("Expected resource %d to be an object", i)
						continue
					}

					resourceFields := []string{"address", "type", "name", "action", "attributes", "raw_lines"}
					for _, field := range resourceFields {
						if _, exists := resMap[field]; !exists {
							t.Errorf("Expected resource %d to have field '%s'", i, field)
						}
					}

					// Validate attributes array structure
					if attrs, ok := resMap["attributes"].([]interface{}); ok {
						for j, attr := range attrs {
							attrMap, ok := attr.(map[string]interface{})
							if !ok {
								t.Errorf("Expected attribute %d of resource %d to be an object", j, i)
								continue
							}

							attrFields := []string{"name", "old_value", "new_value", "action", "computed", "sensitive"}
							for _, field := range attrFields {
								if _, exists := attrMap[field]; !exists {
									t.Errorf("Expected attribute %d of resource %d to have field '%s'", j, i, field)
								}
							}
						}
					}
				}
			}

			// Validate that JSON is reasonable size (not empty, not huge)
			if len(jsonBytes) < 50 {
				t.Error("JSON output seems too small to be a valid plan")
			}
			if len(jsonBytes) > 1024*1024 { // 1MB limit for test data
				t.Error("JSON output seems unreasonably large")
			}
		})
	}
}

// TestJSONSchemaConsistency ensures the JSON output matches expected schema
func TestJSONSchemaConsistency(t *testing.T) {
	// Test with a known plan to verify exact schema
	input := `
  # aws_instance.web will be created
  + resource "aws_instance" "web" {
      + ami           = "ami-12345678"
      + instance_type = "t2.micro"
    }

Plan: 1 to add, 0 to change, 0 to destroy.
`

	plan, err := Parse(input)
	if err != nil {
		t.Fatalf("Failed to parse plan: %v", err)
	}

	jsonBytes, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("Failed to marshal plan to JSON: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// Validate exact schema structure matches architecture document
	expected := map[string]string{
		"resources":      "array",
		"summary":        "string",
		"total_add":      "number",
		"total_change":   "number",
		"total_destroy":  "number",
		"output_count":   "number",
		"raw_plan":       "string",
	}

	for field, expectedType := range expected {
		value, exists := result[field]
		if !exists {
			t.Errorf("Required field '%s' missing from JSON output", field)
			continue
		}

		var actualType string
		switch value.(type) {
		case string:
			actualType = "string"
		case float64:
			actualType = "number"
		case []interface{}:
			actualType = "array"
		case map[string]interface{}:
			actualType = "object"
		case bool:
			actualType = "boolean"
		default:
			actualType = "unknown"
		}

		if actualType != expectedType {
			t.Errorf("Field '%s' expected to be %s, got %s", field, expectedType, actualType)
		}
	}

	// Validate resource structure
	resources := result["resources"].([]interface{})
	if len(resources) > 0 {
		res := resources[0].(map[string]interface{})
		resourceFields := map[string]string{
			"address":    "string",
			"type":       "string",
			"name":       "string",
			"action":     "string",
			"attributes": "array",
			"raw_lines":  "array",
		}

		for field, expectedType := range resourceFields {
			value, exists := res[field]
			if !exists {
				t.Errorf("Required resource field '%s' missing", field)
				continue
			}

			var actualType string
			switch value.(type) {
			case string:
				actualType = "string"
			case []interface{}:
				actualType = "array"
			default:
				actualType = "unknown"
			}

			if actualType != expectedType {
				t.Errorf("Resource field '%s' expected to be %s, got %s", field, expectedType, actualType)
			}
		}
	}
}
