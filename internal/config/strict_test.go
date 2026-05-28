package config

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestDetectSecretLike(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "github pat", value: "ghp_0123456789abcdefghijABCDEFGHIJ012345", want: true},
		{name: "openai", value: "sk-0123456789abcdefABCDxyz", want: true},
		{name: "aws", value: "AKIAIOSFODNN7EXAMPLE", want: true},
		{name: "slack", value: "xoxb-canary-not-a-real-token-000000", want: true},
		{name: "jwt", value: "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig", want: true},
		{name: "age", value: "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p", want: true},
		{name: "plain word", value: "production", want: false},
		{name: "log level", value: "info", want: false},
		{name: "image tag", value: "v1-0-11", want: false},
		{name: "short", value: "abc123", want: false},
		{name: "url", value: "https://app.example.com", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := DetectSecretLike(tt.value)
			if got != tt.want {
				e := shannonEntropy(tt.value)
				t.Errorf("DetectSecretLike(%q) = %v, want %v (entropy %.2f)", tt.value, got, tt.want, e)
			}
		})
	}
}

// TestValidateStrictEntropyDetection asserts custom-format high-entropy secrets in
// visible values are flagged by --strict.
func TestValidateStrictEntropyDetection(t *testing.T) {
	for i := 1; i <= 5; i++ {
		path := filepath.Join("testdata", fmt.Sprintf("custom-secret-%d.yaml", i))
		t.Run(filepath.Base(path), func(t *testing.T) {
			rep, err := Validate(path, true)
			if err != nil {
				t.Fatal(err)
			}
			if rep.OK() {
				t.Fatalf("%s: expected a strict issue, got none", path)
			}
		})
	}
}

// TestValidateStrictNoFalsePositive ensures the entropy heuristic does not flag the
// ordinary values used in the minimal example.
func TestValidateStrictNoFalsePositive(t *testing.T) {
	rep, err := Validate(filepath.Join("..", "..", "examples", "minimal"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Errorf("minimal example flagged under --strict: %+v", rep.Issues)
	}
}
