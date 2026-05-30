package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func beenaire() string { return filepath.Join("..", "..", "examples", "beenaire") }

func envInterpApp() string {
	return filepath.Join("..", "..", "examples", "env-interp", "application.yaml")
}

// TestLoadLenient_NoEnvRequired is the W5c dogfood regression: loading the Beenaire example
// with no secret env var set must succeed, every typed secret keeping its ${env:} origin
// without a value. Read-only flows (validate, plan, explore) depend on this.
func TestLoadLenient_NoEnvRequired(t *testing.T) {
	apps, err := LoadApplications(beenaire())
	if err != nil {
		t.Fatalf("LoadApplications with no env set must succeed: %v", err)
	}
	if len(apps) == 0 {
		t.Fatal("no applications loaded from the beenaire example")
	}
	var secretsSeen int
	for _, app := range apps {
		for _, e := range app.Spec.EnvVars {
			if e.ValueSecret.IsZero() {
				continue
			}
			secretsSeen++
			if !e.ValueSecret.IsUnresolvedEnv() {
				t.Errorf("%s/%s: secret resolved at load, want deferred to apply", app.Metadata.Name, e.Name)
			}
			if e.ValueSecret.Origin() == "" {
				t.Errorf("%s/%s: secret lost its ${env:} origin at load", app.Metadata.Name, e.Name)
			}
		}
	}
	if secretsSeen == 0 {
		t.Fatal("expected at least one ${env:} secret in the beenaire example")
	}
}

// TestLoadLenient_PlainEnvKeptLiteral asserts a visible (Param) ${env:} reference is kept
// literal at load, with no env var required; the value is bound only at apply.
func TestLoadLenient_PlainEnvKeptLiteral(t *testing.T) {
	app, err := LoadApplication(envInterpApp())
	if err != nil {
		t.Fatalf("LoadApplication with no env set must succeed: %v", err)
	}
	if !strings.Contains(app.Spec.FQDN, "${env:") {
		t.Errorf("fqdn = %q, want an unresolved ${env:} reference kept literal", app.Spec.FQDN)
	}
}

// TestResolveSecrets_BindsEnv asserts the explicit apply-time pass resolves both a typed
// env secret and a visible env reference, leaving the origin intact.
func TestResolveSecrets_BindsEnv(t *testing.T) {
	t.Setenv("DATABASE_URL_STAGING", "postgres://x")
	t.Setenv("DJANGO_SECRET_KEY_STAGING", "django-key")

	apps, err := LoadApplications(beenaire())
	if err != nil {
		t.Fatal(err)
	}
	if err := ResolveSecrets(apps, nil, nil); err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	for _, app := range apps {
		for _, e := range app.Spec.EnvVars {
			if e.ValueSecret.IsZero() {
				continue
			}
			if e.ValueSecret.IsUnresolvedEnv() {
				t.Errorf("%s/%s: secret still unresolved after ResolveSecrets", app.Metadata.Name, e.Name)
			}
			if e.ValueSecret.Origin() == "" {
				t.Errorf("%s/%s: secret lost its origin during resolution", app.Metadata.Name, e.Name)
			}
		}
	}
}

// TestResolveSecrets_PlainEnvResolved asserts a visible env reference is interpolated by the
// apply-time pass.
func TestResolveSecrets_PlainEnvResolved(t *testing.T) {
	t.Setenv("APP_NAME", "web")
	t.Setenv("APP_PROJECT", "beenaire")
	t.Setenv("APP_ENV", "staging")
	t.Setenv("IMAGE_REGISTRY", "registry.example.com/app")
	t.Setenv("IMAGE_TAG", "v1")
	t.Setenv("PUBLIC_HOST", "app.example.com")
	t.Setenv("DEPLOY_SERVER", "localhost")
	t.Setenv("APP_LOG_LEVEL", "info")

	app, err := LoadApplication(envInterpApp())
	if err != nil {
		t.Fatal(err)
	}
	apps := []resource.Application{app}
	if err := ResolveSecrets(apps, nil, nil); err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if strings.Contains(apps[0].Spec.FQDN, "${env:") {
		t.Errorf("fqdn = %q, want the ${env:} reference resolved", apps[0].Spec.FQDN)
	}
}

// TestResolveSecrets_MissingEnvErrors asserts apply fails clearly — naming the resource —
// when a referenced env var is unset, rather than pushing an empty value.
func TestResolveSecrets_MissingEnvErrors(t *testing.T) {
	apps, err := LoadApplications(beenaire())
	if err != nil {
		t.Fatal(err)
	}
	err = ResolveSecrets(apps, nil, nil)
	if err == nil {
		t.Fatal("ResolveSecrets with unset env vars must error")
	}
	if !strings.Contains(err.Error(), "application") {
		t.Errorf("error = %q, want it to name the offending application", err)
	}
}
