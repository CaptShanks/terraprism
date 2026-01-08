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
