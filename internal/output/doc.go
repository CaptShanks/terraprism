// Package output handles JSON serialization of Terraform plan data
// for machine-readable output via the --json flag.
//
// This package converts the structured plan data from internal/parser
// into stable JSON schemas suitable for programmatic consumption by
// CI/CD pipelines and automation tools.
package output