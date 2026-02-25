package tui

import (
	"testing"
)

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		text   string
		query  string
		expect bool
	}{
		{"aws_lambda_function.example", "lambda", true},
		{"aws_lambda_function.example", "lmbda", true},
		{"aws_lambda_function.example", "lam", true},
		{"aws_instance.main", "inst", true},
		{"aws_instance.main", "ai", true},
		{"module.foo.aws_s3_bucket.bar", "s3", true},
		{"module.foo.aws_s3_bucket.bar", "s3b", true},
		{"aws_instance.main", "xyz", false},
		{"lambda", "lmbda", true},
		{"lambda", "lmbdx", false},
		{"", "a", false},
		{"abc", "", true},
	}
	for _, tt := range tests {
		got := fuzzyMatch(tt.text, tt.query)
		if got != tt.expect {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tt.text, tt.query, got, tt.expect)
		}
	}
}
