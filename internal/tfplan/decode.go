package tfplan

import (
	"encoding/json"
	"fmt"
	"io"

	tfjson "github.com/hashicorp/terraform-json"
)

// Decode reads and decodes a `terraform show -json` (or `tofu show -json`)
// plan document from r into the terraprism view model.
func Decode(r io.Reader) (*Plan, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading plan JSON: %w", err)
	}
	return DecodeBytes(raw)
}

// DecodeBytes decodes a `terraform show -json` plan document already held
// in memory. Numbers are decoded as json.Number to avoid precision loss
// on large integers (e.g. AWS account IDs). FormatVersion compatibility
// is enforced by terraform-json's own Plan.Validate (constraint
// ">= 0.1, < 2.0"), invoked automatically during unmarshal.
func DecodeBytes(raw []byte) (*Plan, error) {
	var tfPlan tfjson.Plan
	tfPlan.UseJSONNumber(true)
	if err := json.Unmarshal(raw, &tfPlan); err != nil {
		return nil, fmt.Errorf("decoding terraform plan JSON: %w", err)
	}
	return convertPlan(&tfPlan, raw)
}
