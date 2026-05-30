package tui

import (
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func TestServiceDetail_MaskedByDefault(t *testing.T) {
	d := serviceDetail("kafka", []coolify.ServiceEnvVar{
		{Key: "SASL_PASSWORD", Value: "s3cr3t"},
		{Key: "KAFKA_PORT", Value: "9092"},
	})
	if !d.hasEnvs() {
		t.Fatal("service detail has no env table")
	}
	// Masked by default: neither value is shown.
	for _, e := range d.envs {
		if got := d.renderEnvValue(e.value); got != mask {
			t.Errorf("default render of %q = %q, want mask", e.key, got)
		}
	}
}

func TestServiceDetail_RevealTogglesValue(t *testing.T) {
	d := serviceDetail("kafka", []coolify.ServiceEnvVar{{Key: "SASL_PASSWORD", Value: "s3cr3t"}})

	if got := d.renderEnvValue("s3cr3t"); got != mask {
		t.Fatalf("before reveal = %q, want mask", got)
	}
	d.revealed = true
	if got := d.renderEnvValue("s3cr3t"); got != "s3cr3t" {
		t.Fatalf("after reveal = %q, want cleartext", got)
	}
	d.revealed = false
	if got := d.renderEnvValue("s3cr3t"); got != mask {
		t.Fatalf("after re-hide = %q, want mask", got)
	}
}

func TestApplicationDetail_StructFieldsNoEnvTable(t *testing.T) {
	d := applicationDetail(coolify.Application{
		UUID: "a1", Name: "web", Status: "running", BuildPack: "nixpacks",
		DockerRegistryImageName: "ghcr.io/acme/web", DockerRegistryImageTag: "v1",
	})
	if d.kind != resource.KindApplication {
		t.Fatalf("kind = %q, want Application", d.kind)
	}
	if d.hasEnvs() {
		t.Fatal("application detail must not carry an env table")
	}
	joined := fieldString(d)
	for _, want := range []string{"a1", "running", "nixpacks", "ghcr.io/acme/web:v1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("application fields missing %q in %q", want, joined)
		}
	}
}

func TestDatabaseDetail_StructFieldsNoEnvTable(t *testing.T) {
	d := databaseDetail(coolify.Database{UUID: "db1", Name: "redis", Status: "running", DatabaseType: "standalone-redis", IsPublic: true, PublicPort: 6379})
	if d.hasEnvs() {
		t.Fatal("database detail must not carry an env table")
	}
	joined := fieldString(d)
	for _, want := range []string{"db1", "standalone-redis", "true", "6379"} {
		if !strings.Contains(joined, want) {
			t.Errorf("database fields missing %q in %q", want, joined)
		}
	}
}

func fieldString(d detail) string {
	var parts []string
	for _, f := range d.fields {
		parts = append(parts, f.label+"="+f.value)
	}
	return strings.Join(parts, " ")
}
