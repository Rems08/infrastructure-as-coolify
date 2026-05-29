package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/cmd/sops/common"
	"github.com/getsops/sops/v3/config"
	yamlstore "github.com/getsops/sops/v3/stores/yaml"
	sopsversion "github.com/getsops/sops/v3/version"
)

// testAgeRecipient is the public recipient fixtures are encrypted to; the matching private
// identity is written to a temp key file pointed at by SOPS_AGE_KEY_FILE. Both are
// generated fresh per test run by TestMain — no key material is ever committed.
var testAgeRecipient string

// TestMain generates an ephemeral age key pair, exposes the private half via
// SOPS_AGE_KEY_FILE (mode 0600), and tears it down after the run.
func TestMain(m *testing.M) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		panic(err)
	}
	testAgeRecipient = id.Recipient().String()

	tmpDir, err := os.MkdirTemp("", "iac-coolify-sops-test-*")
	if err != nil {
		panic(err)
	}
	keyFile := filepath.Join(tmpDir, "keys.txt")
	if err := os.WriteFile(keyFile, []byte(id.String()+"\n"), 0o600); err != nil {
		panic(err)
	}
	if err := os.Setenv("SOPS_AGE_KEY_FILE", keyFile); err != nil {
		panic(err)
	}

	code := m.Run()

	_ = os.Unsetenv("SOPS_AGE_KEY_FILE")
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

// encryptForTest encrypts plain YAML to recipient and returns SOPS-wrapped ciphertext,
// mirroring what `sops --encrypt` produces, without shelling out to the sops binary.
func encryptForTest(t *testing.T, recipient string, plain []byte) []byte {
	t.Helper()
	masterKey, err := sopsage.MasterKeyFromRecipient(recipient)
	if err != nil {
		t.Fatalf("age master key: %v", err)
	}
	store := yamlstore.NewStore(&config.YAMLStoreConfig{})
	branches, err := store.LoadPlainFile(plain)
	if err != nil {
		t.Fatalf("load plain: %v", err)
	}
	tree := sops.Tree{
		Branches: branches,
		Metadata: sops.Metadata{
			KeyGroups: []sops.KeyGroup{{masterKey}},
			Version:   sopsversion.Version,
		},
	}
	dataKey, errs := tree.GenerateDataKey()
	if len(errs) > 0 {
		t.Fatalf("generate data key: %v", errs)
	}
	if encErr := common.EncryptTree(common.EncryptTreeOpts{
		Tree: &tree, Cipher: aes.NewCipher(), DataKey: dataKey,
	}); encErr != nil {
		t.Fatalf("encrypt tree: %v", encErr)
	}
	out, err := store.EmitEncryptedFile(tree)
	if err != nil {
		t.Fatalf("emit encrypted: %v", err)
	}
	return out
}

const fixturePlaintext = `databases:
  staging:
    password: super-secret-fixture-value
`

// writeFixture encrypts the standard fixture into <dir>/secrets.enc.yaml and returns a
// manifest path colocated with it (the file itself need not exist).
func writeFixture(t *testing.T, dir, recipient string) string {
	t.Helper()
	enc := encryptForTest(t, recipient, []byte(fixturePlaintext))
	if err := os.WriteFile(filepath.Join(dir, sopsFileName), enc, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return filepath.Join(dir, "coolify.yaml")
}

func TestSOPSDecryptValid(t *testing.T) {
	dir := t.TempDir()
	iacPath := writeFixture(t, dir, testAgeRecipient)

	sec, err := LoadSOPSValue(iacPath, "databases.staging.password")
	if err != nil {
		t.Fatalf("LoadSOPSValue: %v", err)
	}
	if got := sec.Reveal(); got != "super-secret-fixture-value" {
		t.Errorf("Reveal() = %q, want fixture value", got)
	}
	if got := sec.Origin(); got != "${sops:databases.staging.password}" {
		t.Errorf("Origin() = %q", got)
	}
	if sec.IsPendingSOPS() {
		t.Error("resolved secret must not be pending")
	}
	if sec.String() != "[REDACTED]" {
		t.Errorf("String() = %q, want [REDACTED]", sec.String())
	}
}

func TestSOPSDecryptFileMissing(t *testing.T) {
	dir := t.TempDir() // no secrets.enc.yaml written
	iacPath := filepath.Join(dir, "coolify.yaml")

	_, err := LoadSOPSValue(iacPath, "databases.staging.password")
	if err == nil {
		t.Fatal("expected an error when secrets.enc.yaml is absent")
	}
	if got := err.Error(); !strings.Contains(got, sopsFileName) || !strings.Contains(got, "not found") {
		t.Errorf("error = %q, want mention of missing %s", got, sopsFileName)
	}
}

func TestSOPSDecryptKeyNotFound(t *testing.T) {
	dir := t.TempDir()
	iacPath := writeFixture(t, dir, testAgeRecipient)

	_, err := LoadSOPSValue(iacPath, "databases.production.password")
	if err == nil {
		t.Fatal("expected an error for a path absent from the document")
	}
	if got := err.Error(); !strings.Contains(got, "not found") {
		t.Errorf("error = %q, want 'key ... not found'", got)
	}
}

func TestSOPSDecryptFailed(t *testing.T) {
	dir := t.TempDir()
	// Encrypt to the run's recipient, then point SOPS at a different identity that cannot
	// decrypt it.
	iacPath := writeFixture(t, dir, testAgeRecipient)

	other, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	otherKeyFile := filepath.Join(t.TempDir(), "other.txt")
	if wErr := os.WriteFile(otherKeyFile, []byte(other.String()+"\n"), 0o600); wErr != nil {
		t.Fatal(wErr)
	}
	t.Setenv("SOPS_AGE_KEY_FILE", otherKeyFile)

	_, err = LoadSOPSValue(iacPath, "databases.staging.password")
	if err == nil {
		t.Fatal("expected a decrypt failure with a non-matching age key")
	}
	if got := err.Error(); !strings.Contains(got, "decrypt") {
		t.Errorf("error = %q, want mention of decrypt", got)
	}
}

func TestSOPSRejectsLooseKeyFilePermissions(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "keys.txt")
	if err := os.WriteFile(keyFile, []byte("AGE-SECRET-KEY-FIXTURE\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOPS_AGE_KEY_FILE", keyFile)

	tests := []struct {
		name    string
		mode    os.FileMode
		wantErr bool
	}{
		{name: "group-readable 0640", mode: 0o640, wantErr: true},
		{name: "world-readable 0644", mode: 0o644, wantErr: true},
		{name: "owner-only 0600", mode: 0o600, wantErr: false},
		{name: "read-only 0400", mode: 0o400, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Chmod(keyFile, tt.mode); err != nil {
				t.Fatal(err)
			}
			err := checkAgeKeyFilePermissions()
			if tt.wantErr && err == nil {
				t.Errorf("mode %#o: want error, got nil", tt.mode)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("mode %#o: unexpected error: %v", tt.mode, err)
			}
		})
	}
}

func TestLoadSOPSValueRejectsLooseKeyPerms(t *testing.T) {
	dir := t.TempDir()
	iacPath := writeFixture(t, dir, testAgeRecipient)

	looseKey := filepath.Join(t.TempDir(), "keys.txt")
	if err := os.WriteFile(looseKey, []byte("AGE-SECRET-KEY-FIXTURE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOPS_AGE_KEY_FILE", looseKey)

	_, err := LoadSOPSValue(iacPath, "databases.staging.password")
	if err == nil {
		t.Fatal("want error: loose key-file permissions must be refused before decrypt")
	}
	if !strings.Contains(err.Error(), "group/other-accessible") {
		t.Errorf("error = %q, want key-permission rejection", err)
	}
}

func TestLookupYAMLPathErrors(t *testing.T) {
	doc := []byte("a:\n  b: leaf\nc: scalar\n")
	tests := []struct{ name, path string }{
		{name: "intermediate is scalar", path: "c.x"},
		{name: "missing key", path: "a.z"},
		{name: "leaf is mapping", path: "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := lookupYAMLPath(doc, tt.path); err == nil {
				t.Errorf("path %q: want error, got nil", tt.path)
			}
		})
	}
	t.Run("malformed document", func(t *testing.T) {
		if _, err := lookupYAMLPath([]byte("[1, 2, 3]"), "a"); err == nil {
			t.Error("want parse error for a non-mapping document")
		}
	})
	t.Run("valid scalar", func(t *testing.T) {
		got, err := lookupYAMLPath(doc, "a.b")
		if err != nil || got != "leaf" {
			t.Errorf("lookupYAMLPath = %q, %v; want \"leaf\", nil", got, err)
		}
	})
}

func TestAgeKeyFilePathDefault(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY_FILE", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	want := filepath.Join(home, ".config", "sops", "age", "keys.txt")
	if got := ageKeyFilePath(); got != want {
		t.Errorf("ageKeyFilePath() = %q, want %q", got, want)
	}
}

func TestCheckAgeKeyFilePermissionsAbsent(t *testing.T) {
	t.Setenv("SOPS_AGE_KEY_FILE", filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err := checkAgeKeyFilePermissions(); err != nil {
		t.Errorf("absent key file must not error (SOPS_AGE_KEY may carry the key): %v", err)
	}
}
