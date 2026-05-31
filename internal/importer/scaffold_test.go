package importer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ScaffoldRoot writes coolify.yaml when absent and is a no-op (wrote=false, no overwrite) when
// the root already exists.
func TestScaffoldRoot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coolify.yaml")

	wrote, err := ScaffoldRoot(dir, "https://coolify.example")
	if err != nil || !wrote {
		t.Fatalf("ScaffoldRoot(absent) = %v, %v; want true, nil", wrote, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"api_version:", "required_coolify:", "https://coolify.example"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("scaffolded root missing %q:\n%s", want, data)
		}
	}

	if wErr := os.WriteFile(path, []byte("pre-existing\n"), 0o600); wErr != nil {
		t.Fatal(wErr)
	}
	wrote, err = ScaffoldRoot(dir, "https://coolify.example")
	if err != nil || wrote {
		t.Fatalf("ScaffoldRoot(present) = %v, %v; want false, nil", wrote, err)
	}
	if got, _ := os.ReadFile(path); string(got) != "pre-existing\n" {
		t.Error("ScaffoldRoot overwrote an existing root manifest")
	}
}

// A conflict surfaces as ErrManifestsExist so callers can detect it without parsing the message.
func TestRun_ConflictIsErrManifestsExist(t *testing.T) {
	dir := t.TempDir()
	collision := filepath.Join(dir, "environments/staging/applications/api.yaml")
	if err := os.MkdirAll(filepath.Dir(collision), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(collision, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), newFake(), Options{Dir: dir, DefaultNetwork: "coolify"})
	if !errors.Is(err, ErrManifestsExist) {
		t.Fatalf("Run conflict error = %v, want ErrManifestsExist", err)
	}
}
