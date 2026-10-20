// Package tfplan decodes `terraform show -json` (or `tofu show -json`)
// plan output into a structured view model that internal/tui renders
// directly, with no text or regex parsing involved.
//
// Decoding happens in two steps: Decode/DecodeBytes unmarshal the raw
// JSON into github.com/hashicorp/terraform-json's canonical types, then
// convert.go walks each resource's before/after/after_unknown/
// before_sensitive/after_sensitive value trees in parallel to build a
// single, pre-diffed Attribute tree per resource (see sensitivity.go for
// the sensitive/unknown path-resolution logic). internal/tui never sees
// the raw JSON shape or terraform-json types.
package tfplan
