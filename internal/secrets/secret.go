// Package secrets provides the opaque Secret type and env-var sourcing used
// throughout iac-coolify. A Secret never appears in logs, plan/apply output,
// state cache, error wrapping, or any string representation other than
// "[REDACTED]". The only escape hatch is Reveal(), restricted by an AST ratchet
// (reveal_lint_test.go) to internal/secrets/ and internal/coolify/.
package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/goccy/go-yaml"
)

// Source identifies where a Secret was loaded from. Used in audit log + diff display.
type Source uint8

const (
	// SourceUnset is the zero value: an unset Secret.
	SourceUnset Source = iota
	// SourceEnv marks a Secret sourced from ${env:VAR}.
	SourceEnv
	// SourceSOPS marks a Secret sourced from ${sops:path}.
	SourceSOPS
)

// Secret holds a value that MUST NEVER appear in logs, plan output, state cache,
// error wrapping, or any string representation other than "[REDACTED]".
type Secret struct {
	value  string // unexported — no reflection access by design
	source Source
	origin string // "${env:DATABASE_URL}" or "${sops:stripe.key}" — safe to log
	// pending marks a ${sops:path} reference parsed but not yet decrypted. SOPS resolution
	// needs the manifest's directory to find the colocated secrets.enc.yaml, which is only
	// known at config-load time — so the value is filled in a second pass, not at decode.
	pending bool
}

// NewFromEnv builds a Secret from an env var (looked up at construction time).
// It returns an error if the env var is absent OR empty (no silent fallback).
func NewFromEnv(envName string) (Secret, error) {
	v, ok := os.LookupEnv(envName)
	if !ok {
		return Secret{}, fmt.Errorf("secrets: env var %q not set", envName)
	}
	if v == "" {
		return Secret{}, fmt.Errorf("secrets: env var %q is empty", envName)
	}
	return Secret{value: v, source: SourceEnv, origin: "${env:" + envName + "}"}, nil
}

// NewFromSOPS builds a Secret from a SOPS-decrypted value.
func NewFromSOPS(decrypted, path string) Secret {
	return Secret{value: decrypted, source: SourceSOPS, origin: "${sops:" + path + "}"}
}

// Reveal returns the raw value. The caller MUST use it immediately at a security
// boundary (HTTP header, API request body) and MUST NOT pass it to fmt/log/marshal.
// Call sites are restricted by reveal_lint_test.go.
func (s Secret) Reveal() string { return s.value }

// Origin returns the source declaration (e.g. "${env:DATABASE_URL}"). Safe to log.
func (s Secret) Origin() string { return s.origin }

// ValueEquals reports whether two secrets hold the same underlying value, without
// revealing it. The diff engine uses this for Notify-only secret comparison:
// it learns a value changed without ever seeing the value.
func (s Secret) ValueEquals(other Secret) bool { return s.value == other.value }

// IsZero reports whether the Secret is unset.
func (s Secret) IsZero() bool { return s.source == SourceUnset }

// hash returns sha256(value) hex-truncated to 8 chars. Used by the diff engine to
// detect value changes when the source is unchanged. NEVER printed in plan output.
func (s Secret) hash() string {
	sum := sha256.Sum256([]byte(s.value))
	return hex.EncodeToString(sum[:])[:8]
}

// --- Interfaces redact: ALL string-y representations return REDACTED ---

// String implements fmt.Stringer.
func (s Secret) String() string { return "[REDACTED]" }

// GoString implements fmt.GoStringer.
func (s Secret) GoString() string { return "secrets.Secret{[REDACTED]}" }

// MarshalJSON implements json.Marshaler.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

// MarshalYAML implements the goccy/go-yaml InterfaceMarshaler: it serialises the
// origin declaration only, never the value.
func (s Secret) MarshalYAML() (any, error) { return s.origin, nil }

// LogValue implements slog.LogValuer — automatic redaction in slog handlers.
func (s Secret) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

// UnmarshalYAML implements the goccy/go-yaml BytesUnmarshaler. It accepts a string
// that MUST match ${env:NAME} or ${sops:path}; literal values are rejected.
func (s *Secret) UnmarshalYAML(b []byte) error {
	var raw string
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return err
	}
	return s.parse(raw)
}

func (s *Secret) parse(raw string) error {
	if name, ok := envRef(raw); ok {
		sec, err := NewFromEnv(name)
		if err != nil {
			return err
		}
		*s = sec
		return nil
	}
	if path, ok := sopsRef(raw); ok {
		*s = Secret{source: SourceSOPS, origin: "${sops:" + path + "}", pending: true}
		return nil
	}
	return fmt.Errorf(
		"secrets: literal value forbidden, use ${env:NAME} or ${sops:path}, got: %s",
		redactPreview(raw),
	)
}

// sopsRef extracts the dotted path from "${sops:path}", returning ok=false otherwise.
func sopsRef(raw string) (string, bool) {
	if !strings.HasPrefix(raw, "${sops:") || !strings.HasSuffix(raw, "}") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(raw, "${sops:"), "}"), true
}

// IsPendingSOPS reports whether s is a ${sops:path} reference awaiting decryption.
func (s Secret) IsPendingSOPS() bool { return s.pending }

// SOPSPath returns the dotted key of a pending SOPS reference (e.g. "db.password").
func (s Secret) SOPSPath() string {
	path, _ := sopsRef(s.origin)
	return path
}

// envRef extracts NAME from "${env:NAME}", returning ok=false for any other input.
func envRef(raw string) (string, bool) {
	if !strings.HasPrefix(raw, "${env:") || !strings.HasSuffix(raw, "}") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(raw, "${env:"), "}"), true
}

// redactPreview shows the first 3 chars + length so a parse error is debuggable
// without leaking the full forbidden literal.
func redactPreview(raw string) string {
	if len(raw) <= 3 {
		return "***"
	}
	return raw[:3] + fmt.Sprintf("***(%d chars)", len(raw))
}
