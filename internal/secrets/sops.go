package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getsops/sops/v3/decrypt"
	"github.com/goccy/go-yaml"
)

// sopsFileName is the encrypted file expected alongside any IaC YAML that references a
// ${sops:path} value. Keeping the path implicit (colocated, never user-supplied) removes
// any path-traversal surface: a manifest can only read secrets shipped next to it.
const sopsFileName = "secrets.enc.yaml"

// LoadSOPSValue resolves a ${sops:path} reference. iacFilePath is the manifest that
// declared the reference; the encrypted store is its colocated secrets.enc.yaml.
// internalPath is the dotted key (e.g. "databases.staging.password") looked up in the
// decrypted document. The returned Secret carries only the decrypted value and the
// "${sops:...}" origin — never the path of the key file or the ciphertext.
func LoadSOPSValue(iacFilePath, internalPath string) (Secret, error) {
	encPath := filepath.Join(filepath.Dir(iacFilePath), sopsFileName)
	if _, err := os.Stat(encPath); err != nil {
		return Secret{}, fmt.Errorf(
			"secrets: %s not found next to %s (SOPS files are colocated with the manifest): %w",
			sopsFileName, filepath.Base(iacFilePath), err,
		)
	}
	if err := checkAgeKeyFilePermissions(); err != nil {
		return Secret{}, err
	}
	plaintext, err := decrypt.File(encPath, "yaml")
	if err != nil {
		return Secret{}, fmt.Errorf("secrets: decrypt %s: %w", encPath, err)
	}
	value, err := lookupYAMLPath(plaintext, internalPath)
	if err != nil {
		return Secret{}, fmt.Errorf("secrets: %s: %w", encPath, err)
	}
	return NewFromSOPS(value, internalPath), nil
}

// ageKeyFilePath returns the age key file SOPS will read: SOPS_AGE_KEY_FILE when set,
// otherwise the conventional ~/.config/sops/age/keys.txt. It returns "" when neither a
// home directory nor the override is available — the caller then skips the permission
// check (SOPS may still resolve the key from the SOPS_AGE_KEY env var instead).
func ageKeyFilePath() string {
	if p := os.Getenv("SOPS_AGE_KEY_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "sops", "age", "keys.txt")
}

// checkAgeKeyFilePermissions refuses an age key file that is group- or world-accessible.
// A private key readable by other accounts on a shared host is the SOPS equivalent of a
// world-readable SSH key. Absence of the file is not an error here: SOPS_AGE_KEY may
// carry the key inline, and a missing key surfaces as a clearer decrypt error downstream.
func checkAgeKeyFilePermissions() error {
	path := ageKeyFilePath()
	if path == "" {
		return nil
	}
	if info, err := os.Stat(path); err == nil && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf(
			"secrets: age key file %s is group/other-accessible (%#o); restrict it to 0600 or 0400",
			path, info.Mode().Perm(),
		)
	}
	return nil
}

// lookupYAMLPath walks a dotted path through a decrypted YAML document and returns the
// scalar string at the leaf. Every failure mode (non-mapping node, missing key,
// non-scalar leaf) yields an explicit error that names the path but never the value.
func lookupYAMLPath(document []byte, dotted string) (string, error) {
	var root map[string]any
	if err := yaml.Unmarshal(document, &root); err != nil {
		return "", fmt.Errorf("parse decrypted document: %w", err)
	}
	segments := strings.Split(dotted, ".")
	var node any = root
	for i, seg := range segments {
		mapping, ok := node.(map[string]any)
		if !ok {
			return "", fmt.Errorf("path %q: %q is not a mapping", dotted, strings.Join(segments[:i], "."))
		}
		node, ok = mapping[seg]
		if !ok {
			return "", fmt.Errorf("path %q: key %q not found", dotted, seg)
		}
	}
	value, ok := node.(string)
	if !ok {
		return "", fmt.Errorf("path %q: value is not a scalar string", dotted)
	}
	return value, nil
}
