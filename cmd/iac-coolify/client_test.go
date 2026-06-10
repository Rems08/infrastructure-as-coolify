package main

import (
	"strings"
	"testing"
)

// IAC_COOLIFY_HTTP_TIMEOUT lets a slow or degraded Coolify instance be reached
// without the 30s default tripping. A valid duration must build a client; an
// unparseable one must fail loudly rather than be silently ignored.
func TestBuildClient_HTTPTimeoutEnv(t *testing.T) {
	t.Run("valid duration builds a client", func(t *testing.T) {
		clearCoolifyEnv(t)
		t.Setenv("COOLIFY_API_URL", "https://coolify.example.com")
		t.Setenv("COOLIFY_API_TOKEN", "tok")
		t.Setenv("IAC_COOLIFY_HTTP_TIMEOUT", "240s")
		c, online, err := buildClient("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !online || c == nil {
			t.Fatalf("want online client, got online=%v client=%v", online, c)
		}
	})

	t.Run("invalid duration is a loud error", func(t *testing.T) {
		clearCoolifyEnv(t)
		t.Setenv("COOLIFY_API_URL", "https://coolify.example.com")
		t.Setenv("COOLIFY_API_TOKEN", "tok")
		t.Setenv("IAC_COOLIFY_HTTP_TIMEOUT", "not-a-duration")
		_, _, err := buildClient("")
		if err == nil {
			t.Fatal("want an error for an unparseable timeout")
		}
		if !strings.Contains(err.Error(), "IAC_COOLIFY_HTTP_TIMEOUT") {
			t.Errorf("error should name the offending variable, got: %v", err)
		}
	})

	t.Run("absent keeps the default", func(t *testing.T) {
		clearCoolifyEnv(t)
		t.Setenv("COOLIFY_API_URL", "https://coolify.example.com")
		t.Setenv("COOLIFY_API_TOKEN", "tok")
		if _, online, err := buildClient(""); err != nil || !online {
			t.Fatalf("want online client with no timeout set, got online=%v err=%v", online, err)
		}
	})
}
