package config

import (
	"strconv"
	"testing"
)

func TestResolveEnvInterpolation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{name: "literal unchanged", raw: "v1-0-11", want: "v1-0-11"},
		{name: "empty unchanged", raw: "", want: ""},
		{
			name: "single env resolved",
			raw:  "${env:NODE_ENV}", env: map[string]string{"NODE_ENV": "production"},
			want: "production",
		},
		{
			name: "multiple env resolved",
			raw:  "${env:A}-${env:B}", env: map[string]string{"A": "x", "B": "y"},
			want: "x-y",
		},
		{
			name: "env inside literal",
			raw:  "https://${env:HOST}/path", env: map[string]string{"HOST": "example.com"},
			want: "https://example.com/path",
		},
		{
			name: "unset env errors",
			raw:  "${env:UNSET_VAR}", wantErr: true,
		},
		{
			name: "one set one unset errors",
			raw:  "${env:SET}-${env:MISSING}", env: map[string]string{"SET": "ok"}, wantErr: true,
		},
		{name: "lowercase name not matched", raw: "${env:lower}", want: "${env:lower}"},
		{name: "spaced name not matched", raw: "${env:bad name}", want: "${env:bad name}"},
		{name: "no brace not matched", raw: "env:FOO", want: "env:FOO"},
		{
			name: "int-castable value",
			raw:  "${env:REPLICAS}", env: map[string]string{"REPLICAS": "5"},
			want: "5",
		},
		{name: "digits-in-name", raw: "${env:VAR1}", env: map[string]string{"VAR1": "z"}, want: "z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := ResolveEnvInterpolation(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveEnvInterpolation(%q): want error, got %q", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveEnvInterpolation(%q): unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ResolveEnvInterpolation(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// TestResolveEnvInterpolation_IntCast verifies the downstream int-parse contract
// (cf. secrets-policy.md §2.2): "${env:REPLICAS}" resolves to "5" then parses to 5.
func TestResolveEnvInterpolation_IntCast(t *testing.T) {
	t.Setenv("REPLICAS", "5")
	got, err := ResolveEnvInterpolation("${env:REPLICAS}")
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", got, err)
	}
	if n != 5 {
		t.Errorf("got %d, want 5", n)
	}
}
