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
