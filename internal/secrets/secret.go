// Package secrets provides the opaque Secret type and env-var sourcing used
// throughout iac-coolify. A Secret never appears in logs, plan/apply output,
// state cache, error wrapping, or any string representation other than
// "[REDACTED]". The only escape hatch is Reveal(), restricted by an AST ratchet
// (reveal_lint_test.go) to internal/secrets/ and internal/coolify/.
package secrets

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	// SourceRemote marks a Secret decoded from a live API response. Its value is a
	// runtime literal with no source declaration; it carries no Origin and exists only
	// so a credential read back from the API stays opaque (redacted by every interface).
	SourceRemote
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

// ResolveEnv resolves a deferred ${env:NAME} reference (origin-only, carrying no value) to a
// concrete Secret by looking the env var up now. It is the apply-time companion to the lenient
// decoder: load keeps the reference, apply binds it to a value. Any Secret that is not an
// unresolved env reference (already resolved, SOPS, remote, or unset) is returned unchanged.
func ResolveEnv(s Secret) (Secret, error) {
	if !s.IsUnresolvedEnv() {
		return s, nil
	}
	name, _ := envRef(s.origin)
	return NewFromEnv(name)
}

// IsUnresolvedEnv reports whether s is an ${env:} reference whose value has not been looked up
// yet. Read-only flows (load, validate, plan, explore) keep secrets in this state; only apply
// resolves them. The diff engine treats an unresolved desired secret as value-unknown so it
// never reports a phantom change against a remote value.
func (s Secret) IsUnresolvedEnv() bool { return s.source == SourceEnv && s.value == "" }

// NewFromSOPS builds a Secret from a SOPS-decrypted value.
func NewFromSOPS(decrypted, path string) Secret {
	return Secret{value: decrypted, source: SourceSOPS, origin: "${sops:" + path + "}"}
}

// NewReference builds a Secret from a source declaration (${env:NAME} or ${sops:path})
// without resolving its value: it carries only its origin, so no env lookup or SOPS
// decryption happens here. The env var need not be set where the manifest is loaded or
// edited, only where it is later applied — load, validate, plan and explore stay lenient,
// and ResolveEnv (env) / NewFromSOPS (sops) supply the value at apply time. A literal
// (anything not matching the two reference forms) is rejected, so a secret can never be
// downgraded to a hardcoded value. This is the decoder used by UnmarshalYAML.
func NewReference(raw string) (Secret, error) {
	if name, ok := envRef(raw); ok {
		return Secret{source: SourceEnv, origin: "${env:" + name + "}"}, nil
	}
	if path, ok := sopsRef(raw); ok {
		return Secret{source: SourceSOPS, origin: "${sops:" + path + "}", pending: true}, nil
	}
	return Secret{}, fmt.Errorf(
		"secrets: literal value forbidden, use ${env:NAME} or ${sops:path}, got: %s",
		redactPreview(raw),
	)
}

// NewRemote builds an opaque Secret from a runtime value read back from the API. It has
// no source declaration (Origin is empty), so it renders as [REDACTED] everywhere. An
// empty value yields the unset zero Secret, so a null/absent API field stays IsZero.
func NewRemote(value string) Secret {
	if value == "" {
		return Secret{}
	}
	return Secret{value: value, source: SourceRemote}
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

// UnmarshalJSON decodes a credential read back from a live API response into an opaque
// remote Secret. Unlike UnmarshalYAML (user input, where a literal is forbidden so secrets
// can never be hardcoded in a manifest), the API legitimately returns the runtime value as
// a JSON literal; it is wrapped immediately so it stays redacted. A JSON null or empty
// string yields the unset zero Secret.
func (s *Secret) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		return nil
	}
	var raw string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*s = NewRemote(raw)
	return nil
}

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

// parse decodes a source declaration into an origin-only Secret. An ${env:} reference keeps
// its origin without an env lookup (the value is resolved later by ResolveEnv, at apply); an
// ${sops:} reference is left pending for the load-time decryption pass. A literal is rejected.
func (s *Secret) parse(raw string) error {
	sec, err := NewReference(raw)
	if err != nil {
		return err
	}
	*s = sec
	return nil
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
