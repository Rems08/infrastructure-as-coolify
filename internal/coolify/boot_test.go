package coolify_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

const pinnedChecksum = "e98fa2b00ce84fb9eae326999c89e6b9d87e96c65528ba7e1da754010cb44413"

func pinnedOpenAPIDir() string { return filepath.Join("..", "..", "testdata", "openapi") }

// TestVerifyBootSpec_OK confirms the pinned in-repo spec passes boot verification.
func TestVerifyBootSpec_OK(t *testing.T) {
	if err := coolify.VerifyBootSpec(pinnedOpenAPIDir()); err != nil {
		t.Fatalf("pinned spec failed boot verify: %v", err)
	}
}

// TestVerifyBootSpec_Absent returns ErrSpecAbsent (not a mismatch) when the dir is empty.
func TestVerifyBootSpec_Absent(t *testing.T) {
	if err := coolify.VerifyBootSpec(t.TempDir()); !errors.Is(err, coolify.ErrSpecAbsent) {
		t.Fatalf("want ErrSpecAbsent for empty dir, got %v", err)
	}
}

// TestBootRefusedOnChecksumMismatch covers critère §7 #27: a tampered spec or a
// stale sidecar must make boot verification fail with an explicit error.
func TestBootRefusedOnChecksumMismatch(t *testing.T) {
	good, err := os.ReadFile(filepath.Join(pinnedOpenAPIDir(), "coolify-v4.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("tampered spec", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "coolify-v4.yaml"), append([]byte("# tampered\n"), good...))
		write(t, filepath.Join(dir, "coolify-v4.yaml.sha256"), []byte(pinnedChecksum+"  coolify-v4.yaml\n"))
		if err := coolify.VerifyBootSpec(dir); err == nil {
			t.Fatal("tampered spec passed boot verify")
		}
	})

	t.Run("stale sidecar", func(t *testing.T) {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "coolify-v4.yaml"), good)
		write(t, filepath.Join(dir, "coolify-v4.yaml.sha256"), []byte("0000000000000000000000000000000000000000000000000000000000000000  coolify-v4.yaml\n"))
		if err := coolify.VerifyBootSpec(dir); err == nil {
			t.Fatal("stale sidecar passed boot verify")
		}
	})
}

func write(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
