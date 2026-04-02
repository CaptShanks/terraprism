package parser

import (
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
