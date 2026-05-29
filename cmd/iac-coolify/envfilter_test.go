package main

import "testing"

func TestMatchesEnv(t *testing.T) {
	tests := []struct {
		name  string
		allow []string
		env   string
		want  bool
	}{
		{name: "empty filter matches any", allow: nil, env: "staging", want: true},
		{name: "empty filter matches empty env", allow: []string{}, env: "", want: true},
		{name: "single match", allow: []string{"staging"}, env: "staging", want: true},
		{name: "single no match", allow: []string{"staging"}, env: "production", want: false},
		{name: "union matches second", allow: []string{"staging", "production"}, env: "production", want: true},
		{name: "union no match", allow: []string{"staging", "production"}, env: "preview", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesEnv(tt.allow, tt.env); got != tt.want {
				t.Errorf("matchesEnv(%v, %q) = %v, want %v", tt.allow, tt.env, got, tt.want)
			}
		})
	}
}
