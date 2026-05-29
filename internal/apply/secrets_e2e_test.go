package apply_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// applySecretApp drives a create for app through the engine with an auditor at auditPath and
// returns the raw audit log plus the decoded single entry.
func applySecretApp(t *testing.T, app resource.Application, auditPath string) (string, apply.AuditEntry) {
	t.Helper()
	resolved := state.Map{
		state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}: "proj-uuid",
		state.ResourceKey{Kind: state.KindServer, Name: "localhost"}:    "srv-uuid",
	}
	eng := apply.NewEngine(&mockClient{}, resolved, apply.NewAuditor(auditPath))
	if _, err := eng.Apply(context.Background(), []apply.Operation{apply.ApplicationOp(apply.OpCreate, app, nil)}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimSpace(string(data))
	var e apply.AuditEntry
	if err := json.Unmarshal([]byte(raw), &e); err != nil {
		t.Fatalf("decode audit entry: %v (%s)", err, raw)
	}
	return raw, e
}

func secretAppBase(envVars []resource.EnvVarEntry) resource.Application {
	return resource.Application{
		Metadata: resource.ApplicationMeta{Name: "api", Project: "beenaire", Environment: "staging"},
		Spec: resource.ApplicationSpec{
			BuildPack:   "dockerimage",
			Image:       &resource.ImageSpec{Name: "registry/api", Tag: "v1"},
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			Port:        8000,
			EnvVars:     envVars,
		},
	}
}

func TestSecretsLayersE2E(t *testing.T) {
	t.Run("sops-origin secret records origin not value", func(t *testing.T) {
		// A SOPS-origin secret carries its ${sops:...} origin and the underlying value. We
		// assert the value never reaches the audit log while the origin does — the opaque
		// Secret type redacts regardless of how the value was sourced.
		const decrypted = "pg-prod-do-not-leak"
		sec := secrets.NewFromSOPS(decrypted, "databases.staging.password")
		app := secretAppBase([]resource.EnvVarEntry{{Name: "DATABASE_URL", ValueSecret: sec}})

		raw, e := applySecretApp(t, app, filepath.Join(t.TempDir(), "audit.log"))
		if strings.Contains(raw, decrypted) {
			t.Fatal("audit log leaked the decrypted SOPS value")
		}
		if len(e.Sources) != 1 || e.Sources[0] != "${sops:databases.staging.password}" {
			t.Errorf("Sources = %v, want [${sops:databases.staging.password}]", e.Sources)
		}
	})

	t.Run("env Param and secret mixed: sources lists every secret origin", func(t *testing.T) {
		t.Setenv("DB_SECRET", "dbpass-do-not-leak")
		t.Setenv("API_SECRET", "apikey-do-not-leak")
		dbSec, err := secrets.NewFromEnv("DB_SECRET")
		if err != nil {
			t.Fatal(err)
		}
		apiSec, err := secrets.NewFromEnv("API_SECRET")
		if err != nil {
			t.Fatal(err)
		}
		app := secretAppBase([]resource.EnvVarEntry{
			{Name: "NODE_ENV", Value: "production"}, // visible Param, not a secret
			{Name: "DATABASE_URL", ValueSecret: dbSec},
			{Name: "API_KEY", ValueSecret: apiSec},
		})

		raw, e := applySecretApp(t, app, filepath.Join(t.TempDir(), "audit.log"))
		if strings.Contains(raw, "do-not-leak") {
			t.Fatal("audit log leaked a secret value")
		}
		if len(e.Sources) != 2 {
			t.Fatalf("Sources = %v, want both secret origins", e.Sources)
		}
		joined := strings.Join(e.Sources, ",")
		if !strings.Contains(joined, "${env:DB_SECRET}") || !strings.Contains(joined, "${env:API_SECRET}") {
			t.Errorf("Sources = %v, want both env origins", e.Sources)
		}
	})

	t.Run("audit entry is enriched and the log is 0600", func(t *testing.T) {
		t.Setenv("IAC_COOLIFY_ACTOR", "deployer-9")
		t.Setenv("APP_SECRET", "x-do-not-leak")
		sec, err := secrets.NewFromEnv("APP_SECRET")
		if err != nil {
			t.Fatal(err)
		}
		app := secretAppBase([]resource.EnvVarEntry{{Name: "TOKEN", ValueSecret: sec}})

		auditPath := filepath.Join(t.TempDir(), "audit.log")
		_, e := applySecretApp(t, app, auditPath)
		if len(e.Sources) == 0 {
			t.Error("expected sources to be populated")
		}
		if !strings.HasPrefix(e.DiffHash, "sha256:") {
			t.Errorf("DiffHash = %q, want sha256: digest", e.DiffHash)
		}
		if e.Actor != "deployer-9" {
			t.Errorf("Actor = %q, want deployer-9", e.Actor)
		}
		info, err := os.Stat(auditPath)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("audit log mode = %o, want 600", perm)
		}
	})
}
