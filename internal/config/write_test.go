package config_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

var update = flag.Bool("update", false, "update golden files")

const secretValue = "s3cr3t-django-staging-value"

func TestWriteApplicationRoundTrip(t *testing.T) {
	t.Setenv("APP_SECRET_KEY", secretValue)
	in := filepath.Join("testdata", "write", "application.yaml")

	app, err := config.LoadApplication(in)
	if err != nil {
		t.Fatalf("load input: %v", err)
	}

	out := filepath.Join(t.TempDir(), "application.yaml")
	if err = config.WriteApplication(out, app); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}

	golden := filepath.Join("testdata", "write", "application.golden.yaml")
	if *update {
		if err = os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run `go test -update` to create it): %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("marshalled output differs from golden:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}

	// Load → Write → Load must reproduce the same resource (modulo formatting).
	reloaded, err := config.LoadApplication(out)
	if err != nil {
		t.Fatalf("reload written file: %v", err)
	}
	if !reflect.DeepEqual(app, reloaded) {
		t.Errorf("round-trip changed the resource:\n--- before ---\n%+v\n--- after ---\n%+v", app, reloaded)
	}
}

func TestWriteApplicationDoesNotLeakSecretValue(t *testing.T) {
	t.Setenv("APP_SECRET_KEY", secretValue)
	in := filepath.Join("testdata", "write", "application.yaml")
	app, err := config.LoadApplication(in)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "application.yaml")
	if err = config.WriteApplication(out, app); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), secretValue) {
		t.Error("written manifest leaked the resolved secret value")
	}
	if !strings.Contains(string(got), "${env:APP_SECRET_KEY}") {
		t.Errorf("written manifest dropped the secret source declaration:\n%s", got)
	}
}

func TestWriteApplicationRefusesSecretWithoutOrigin(t *testing.T) {
	app := resource.Application{
		APIVersion: resource.APIVersion,
		Kind:       resource.KindApplication,
		Metadata:   resource.ApplicationMeta{Name: "api", Project: "p", Environment: "staging"},
		Spec: resource.ApplicationSpec{
			BuildPack: "dockerimage",
			EnvVars: []resource.EnvVarEntry{
				// A secret read back from the live API carries no source declaration.
				{Name: "SECRET_KEY", ValueSecret: secrets.NewRemote("leaked-from-api")},
			},
		},
	}
	out := filepath.Join(t.TempDir(), "application.yaml")
	if err := config.WriteApplication(out, app); err == nil {
		t.Fatal("WriteApplication must refuse a secret with no ${env:}/${sops:} declaration")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("WriteApplication must not create a file when it refuses to serialise")
	}
}

func TestWriteDatabaseRoundTrip(t *testing.T) {
	pw, err := secrets.NewReference("${env:PG_RESTAURANT_API_STAGING_PASSWORD}")
	if err != nil {
		t.Fatal(err)
	}
	db := resource.Database{
		APIVersion: resource.APIVersion,
		Kind:       resource.KindDatabase,
		Metadata:   resource.DatabaseMeta{Name: "pg-restaurant-api-staging", Project: "beenaire", Environment: "staging"},
		Spec: resource.DatabaseSpec{
			Engine:      "postgresql",
			Image:       "postgres:18-alpine",
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			Password:    pw,
		},
	}
	out := filepath.Join(t.TempDir(), "db.yaml")
	if err = config.WriteDatabase(out, db); err != nil {
		t.Fatalf("write: %v", err)
	}
	reloaded, err := config.LoadDatabase(out)
	if err != nil {
		t.Fatalf("reload written file: %v", err)
	}
	if vErr := reloaded.Validate(); vErr != nil {
		t.Fatalf("written database does not validate: %v", vErr)
	}
	if !reflect.DeepEqual(db, reloaded) {
		t.Errorf("round-trip changed the database:\n--- before ---\n%+v\n--- after ---\n%+v", db, reloaded)
	}
}

func TestWriteDatabaseRefusesSecretWithoutOrigin(t *testing.T) {
	db := resource.Database{
		APIVersion: resource.APIVersion,
		Kind:       resource.KindDatabase,
		Metadata:   resource.DatabaseMeta{Name: "pg", Project: "p", Environment: "staging"},
		Spec: resource.DatabaseSpec{
			Engine:      "postgresql",
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			// A password read back from the live API carries no source declaration.
			Password: secrets.NewRemote("leaked-from-api"),
		},
	}
	out := filepath.Join(t.TempDir(), "db.yaml")
	if err := config.WriteDatabase(out, db); err == nil {
		t.Fatal("WriteDatabase must refuse a password with no ${env:}/${sops:} declaration")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Error("WriteDatabase must not create a file when it refuses to serialise")
	}
}

func TestWriteDatabaseDoesNotLeakSecretValue(t *testing.T) {
	const leak = "super-secret-pg-password"
	t.Setenv("PG_PASSWORD", leak)
	pw, err := secrets.NewFromEnv("PG_PASSWORD")
	if err != nil {
		t.Fatal(err)
	}
	db := resource.Database{
		APIVersion: resource.APIVersion,
		Kind:       resource.KindDatabase,
		Metadata:   resource.DatabaseMeta{Name: "pg", Project: "p", Environment: "staging"},
		Spec: resource.DatabaseSpec{
			Engine:      "postgresql",
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			Password:    pw,
		},
	}
	out := filepath.Join(t.TempDir(), "db.yaml")
	if err = config.WriteDatabase(out, db); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), leak) {
		t.Error("written manifest leaked the resolved password value")
	}
	if !strings.Contains(string(got), "${env:PG_PASSWORD}") {
		t.Errorf("written manifest dropped the password source declaration:\n%s", got)
	}
}

func TestWriteApplicationPreservesDisabledHealthCheck(t *testing.T) {
	app := resource.Application{
		APIVersion: resource.APIVersion,
		Kind:       resource.KindApplication,
		Metadata:   resource.ApplicationMeta{Name: "api", Project: "p", Environment: "staging"},
		Spec: resource.ApplicationSpec{
			BuildPack:   "dockerimage",
			Image:       &resource.ImageSpec{Name: "registry/api", Tag: "v1"},
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			Port:        8080,
			// enabled:false is significant: a write-back must not drop it via omitempty.
			HealthCheck: &resource.HealthCheckSpec{Enabled: false, Path: "/health"},
		},
	}
	out := filepath.Join(t.TempDir(), "application.yaml")
	if err := config.WriteApplication(out, app); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "enabled: false") {
		t.Errorf("disabled health check was dropped on write:\n%s", got)
	}
	reloaded, err := config.LoadApplication(out)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Spec.HealthCheck == nil || reloaded.Spec.HealthCheck.Enabled {
		t.Errorf("round-trip lost the disabled health check: %+v", reloaded.Spec.HealthCheck)
	}
}
