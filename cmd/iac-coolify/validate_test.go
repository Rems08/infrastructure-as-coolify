package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestValidateCommand_MinimalOK(t *testing.T) {
	out, err := runCmd(t, "validate", filepath.Join("..", "..", "examples", "minimal"))
	if err != nil {
		t.Fatalf("validate minimal: %v (out: %s)", err, out)
	}
	if want := "Validated 1 application: web (no issues)"; !strings.Contains(out, want) {
		t.Errorf("output = %q, want substring %q", out, want)
	}
}

func TestValidateCommand_InvalidFails(t *testing.T) {
	_, err := runCmd(t, "validate", filepath.Join("..", "..", "examples", "invalid"))
	if err == nil {
		t.Fatal("validate invalid: want non-nil error")
	}
}

func TestValidateCommand_StrictCanariesFail(t *testing.T) {
	out, err := runCmd(t, "validate", "--strict",
		filepath.Join("..", "..", "testdata", "secrets-canaris"))
	if err == nil {
		t.Fatal("strict canaries: want non-nil error")
	}
	for _, token := range []string{"ghp_", "sk-", "AKIA", "xoxb-", "eyJ", "age1"} {
		if !strings.Contains(out, token) {
			t.Errorf("strict output missing %q", token)
		}
	}
}

func TestVersionCommand(t *testing.T) {
	out, err := runCmd(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "iac-coolify") {
		t.Errorf("version output = %q", out)
	}
}
