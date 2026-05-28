package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestParseStrictRejectsUnknownFields asserts an unknown YAML field is rejected with its
// name (and position), not silently ignored.
func TestParseStrictRejectsUnknownFields(t *testing.T) {
	_, err := LoadApplication(filepath.Join("testdata", "unknown-field.yaml"))
	if err == nil {
		t.Fatal("expected unknown field to be rejected")
	}
	if !strings.Contains(err.Error(), "build_pack_typo") {
		t.Errorf("error %q does not name the offending field", err)
	}
}

func TestValidateMinimalExample(t *testing.T) {
	rep, err := Validate(filepath.Join("..", "..", "examples", "minimal"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("minimal example has issues: %+v", rep.Issues)
	}
	if len(rep.Apps) != 1 || rep.Apps[0] != "web" {
		t.Errorf("apps = %v, want [web]", rep.Apps)
	}
}

func TestValidateInvalidExample(t *testing.T) {
	rep, err := Validate(filepath.Join("..", "..", "examples", "invalid"), false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() {
		t.Fatal("invalid example should report issues")
	}
}

// TestValidateEnvVarExample asserts a directory mixing an Application and a standalone
// EnvVar validates, and the summary lists both kinds.
func TestValidateEnvVarExample(t *testing.T) {
	rep, err := Validate(filepath.Join("..", "..", "examples", "envvar"), false)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("envvar example has issues: %+v", rep.Issues)
	}
	if len(rep.Apps) != 1 || rep.Apps[0] != "api" {
		t.Errorf("apps = %v, want [api]", rep.Apps)
	}
	if len(rep.EnvVars) != 1 || rep.EnvVars[0] != "app-config" {
		t.Errorf("envvars = %v, want [app-config]", rep.EnvVars)
	}
	var buf strings.Builder
	if !WriteReport(&buf, rep) {
		t.Fatal("WriteReport returned false for a clean report")
	}
	if want := "Validated 1 application + 1 envvar (no issues)"; !strings.Contains(buf.String(), want) {
		t.Errorf("summary = %q, want substring %q", buf.String(), want)
	}
}

// TestValidateLiteralSecretRejected asserts a literal value_secret is rejected at the
// config layer without leaking the literal value.
func TestValidateLiteralSecretRejected(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "secrets-canaris", "literal-rejected.yaml")
	rep, err := Validate(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() {
		t.Fatal("literal value_secret must be rejected")
	}
	joined := rep.Issues[0].Message
	if !strings.Contains(joined, "literal value forbidden") {
		t.Errorf("issue %q missing 'literal value forbidden'", joined)
	}
	if strings.Contains(joined, "user:pass") {
		t.Errorf("issue leaks the literal secret: %q", joined)
	}
}

// TestValidateCanariesDetectsAllSix asserts every known canary prefix is surfaced when
// scanning the secrets-canaris directory under --strict.
func TestValidateCanariesDetectsAllSix(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "secrets-canaris")
	rep, err := Validate(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	for _, iss := range rep.Issues {
		sb.WriteString(iss.Message)
		sb.WriteByte('\n')
	}
	out := sb.String()
	for _, token := range []string{"ghp_", "sk-", "AKIA", "xoxb-", "eyJ", "age1"} {
		if !strings.Contains(out, token) {
			t.Errorf("strict scan output missing canary token %q", token)
		}
	}
}
