package secrets

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestValueEquals(t *testing.T) {
	a := Secret{value: "same", source: SourceEnv, origin: "${env:A}"}
	b := Secret{value: "same", source: SourceEnv, origin: "${env:B}"} // different source, same value
	c := Secret{value: "diff", source: SourceEnv, origin: "${env:A}"}
	if !a.ValueEquals(b) {
		t.Error("equal values must compare equal regardless of origin")
	}
	if a.ValueEquals(c) {
		t.Error("different values must not compare equal")
	}
}

func TestUnmarshalJSON(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantZero   bool
		wantReveal string
	}{
		{name: "literal", in: `"s3cr3t-runtime"`, wantZero: false, wantReveal: "s3cr3t-runtime"},
		{name: "null", in: `null`, wantZero: true},
		{name: "empty", in: `""`, wantZero: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s Secret
			if err := json.Unmarshal([]byte(tt.in), &s); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", tt.in, err)
			}
			if s.IsZero() != tt.wantZero {
				t.Errorf("IsZero() = %v, want %v", s.IsZero(), tt.wantZero)
			}
			if s.Origin() != "" {
				t.Errorf("remote secret must have no origin, got %q", s.Origin())
			}
			if !tt.wantZero && s.Reveal() != tt.wantReveal {
				t.Errorf("Reveal() = %q, want %q", s.Reveal(), tt.wantReveal)
			}
		})
	}
}

func TestUnmarshalJSON_staysRedacted(t *testing.T) {
	var s Secret
	if err := json.Unmarshal([]byte(`"top-secret"`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := s.String(); got != "[REDACTED]" {
		t.Errorf("String() = %q, want [REDACTED]", got)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(out) != `"[REDACTED]"` {
		t.Errorf("MarshalJSON() = %s, want \"[REDACTED]\"", out)
	}
	if strings.Contains(s.String()+string(out), "top-secret") {
		t.Error("value leaked through a string representation")
	}
}

func TestNewFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		setEnv  bool
		wantErr bool
	}{
		{name: "set", envVal: "s3cr3t", setEnv: true, wantErr: false},
		{name: "absent", setEnv: false, wantErr: true},
		{name: "empty", envVal: "", setEnv: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const key = "IAC_TEST_SECRET"
			if tt.setEnv {
				t.Setenv(key, tt.envVal)
			}
			sec, err := NewFromEnv(key)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NewFromEnv(%q): want error, got nil", key)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewFromEnv(%q): unexpected error: %v", key, err)
			}
			if sec.Reveal() != tt.envVal {
				t.Errorf("Reveal() = %q, want %q", sec.Reveal(), tt.envVal)
			}
			if sec.Origin() != "${env:"+key+"}" {
				t.Errorf("Origin() = %q, want %q", sec.Origin(), "${env:"+key+"}")
			}
		})
	}
}

func TestUnmarshalYAML_AcceptsEnvPattern(t *testing.T) {
	cases := []string{
		"DATABASE_URL", "STRIPE_KEY", "A", "A_B", "X1", "FOO_BAR_BAZ",
		"NODE_ENV_STAGING", "_LEADING_UNDERSCORE",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "value-"+name)
			var s Secret
			doc := "value_secret: ${env:" + name + "}\n"
			var holder struct {
				ValueSecret Secret `yaml:"value_secret"`
			}
			if err := yaml.Unmarshal([]byte(doc), &holder); err != nil {
				t.Fatalf("unmarshal %q: %v", doc, err)
			}
			s = holder.ValueSecret
			if s.IsZero() {
				t.Fatal("secret is zero after unmarshal")
			}
			if s.Reveal() != "value-"+name {
				t.Errorf("Reveal() = %q, want %q", s.Reveal(), "value-"+name)
			}
		})
	}
}

func TestUnmarshalYAML_RejectsLiteral(t *testing.T) {
	literals := []string{
		"postgres://user:pass@host/db",
		"plain-string",
		"sk-1234567890",
		"",
		"   ",
		"env:NO_DOLLAR_BRACE",
		"${env:lowercase}", // lowercase env name not matched by NewFromEnv path
		"${ENV:UPPER}",     // wrong prefix case
		"${ env:SPACE }",
		"prefix ${env:X}", // env ref not anchored
	}
	for _, lit := range literals {
		t.Run(lit, func(t *testing.T) {
			var s Secret
			if err := s.parse(lit); err == nil {
				t.Errorf("parse(%q): expected rejection, got nil", lit)
			}
		})
	}
}

// TestParseSOPSRefPending asserts ${sops:path} parses to a pending Secret carrying the
// origin and path, with the value left unresolved until config load supplies the manifest
// directory.
func TestParseSOPSRefPending(t *testing.T) {
	var s Secret
	if err := s.parse("${sops:databases.staging.password}"); err != nil {
		t.Fatalf("parse ${sops:...}: %v", err)
	}
	if !s.IsPendingSOPS() {
		t.Error("expected a pending SOPS secret")
	}
	if s.IsZero() {
		t.Error("a pending SOPS secret must not be zero")
	}
	if got, want := s.SOPSPath(), "databases.staging.password"; got != want {
		t.Errorf("SOPSPath() = %q, want %q", got, want)
	}
	if got := s.Origin(); got != "${sops:databases.staging.password}" {
		t.Errorf("Origin() = %q", got)
	}
	if got := s.String(); got != "[REDACTED]" {
		t.Errorf("pending SOPS secret String() = %q, want [REDACTED]", got)
	}
}

// TestUnmarshalYAMLError asserts a YAML node that cannot decode into a string is rejected.
func TestUnmarshalYAMLError(t *testing.T) {
	var s Secret
	if err := s.UnmarshalYAML([]byte("[1, 2, 3]")); err == nil {
		t.Error("want error unmarshalling a sequence into a Secret")
	}
}

// TestSecretLiteralRejected asserts a literal Secret is rejected and the error message
// NEVER leaks the literal value.
func TestSecretLiteralRejected(t *testing.T) {
	const literal = "postgres://user:pass@host/db"
	var s Secret
	err := s.parse(literal)
	if err == nil {
		t.Fatal("expected literal Secret to be rejected")
	}
	if !strings.Contains(err.Error(), "literal value forbidden") {
		t.Errorf("error message missing 'literal value forbidden': %v", err)
	}
	if strings.Contains(err.Error(), "user:pass") {
		t.Errorf("error message leaks literal secret value: %v", err)
	}
}

// TestAllInterfacesRedact asserts String/GoString/MarshalJSON/MarshalYAML/LogValue
// never expose the value.
func TestAllInterfacesRedact(t *testing.T) {
	t.Setenv("IAC_REDACT_PROBE", "TOPSECRET-VALUE-7f3a")
	sec, err := NewFromEnv("IAC_REDACT_PROBE")
	if err != nil {
		t.Fatal(err)
	}
	const valueProbe = "TOPSECRET-VALUE-7f3a"

	if got := sec.String(); got != "[REDACTED]" {
		t.Errorf("String() = %q, want [REDACTED]", got)
	}
	if got := sec.GoString(); got != "secrets.Secret{[REDACTED]}" {
		t.Errorf("GoString() = %q", got)
	}
	jsonBytes, err := json.Marshal(sec)
	if err != nil {
		t.Fatal(err)
	}
	if string(jsonBytes) != `"[REDACTED]"` {
		t.Errorf("MarshalJSON = %s, want \"[REDACTED]\"", jsonBytes)
	}
	yamlBytes, err := yaml.Marshal(sec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(yamlBytes), "${env:IAC_REDACT_PROBE}") {
		t.Errorf("MarshalYAML = %q, want origin", yamlBytes)
	}
	if got := sec.LogValue().String(); got != "[REDACTED]" {
		t.Errorf("LogValue() = %q, want [REDACTED]", got)
	}

	// Belt-and-braces: no representation may contain the raw value.
	for _, repr := range []string{
		sec.String(), sec.GoString(), string(jsonBytes), string(yamlBytes), sec.LogValue().String(),
	} {
		if strings.Contains(repr, valueProbe) {
			t.Errorf("representation leaks value: %q", repr)
		}
	}
}

func TestRevealReturnsValue(t *testing.T) {
	t.Setenv("IAC_REVEAL_PROBE", "raw-value")
	sec, err := NewFromEnv("IAC_REVEAL_PROBE")
	if err != nil {
		t.Fatal(err)
	}
	if sec.Reveal() != "raw-value" {
		t.Errorf("Reveal() = %q, want raw-value", sec.Reveal())
	}
}

func TestHashStableAndDistinct(t *testing.T) {
	a := NewFromSOPS("alpha", "p.a")
	b := NewFromSOPS("alpha", "p.a")
	c := NewFromSOPS("beta", "p.b")
	if a.hash() != b.hash() {
		t.Error("equal values must hash equally")
	}
	if a.hash() == c.hash() {
		t.Error("distinct values must hash differently")
	}
	if strings.Contains(a.hash(), "alpha") {
		t.Error("hash must not contain the value")
	}
}
