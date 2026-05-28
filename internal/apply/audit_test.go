package apply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

func TestAuditLogPermissions0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".iac-coolify", "audit.log")
	a := apply.NewAuditor(path)
	if err := a.Record(apply.AuditEntry{Resource: "Project/beenaire", Op: "create"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("audit log mode = %o, want 600", perm)
	}
}

func TestAuditLogAppendOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	a := apply.NewAuditor(path)
	for i := 0; i < 3; i++ {
		if err := a.Record(apply.AuditEntry{Resource: "Application/p/s/api", Op: "create"}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; lines != 3 {
		t.Errorf("got %d log lines, want 3 (append-only)", lines)
	}
}

// TestAuditLogNeverContainsSecretValue drives a create through the engine for an
// application carrying a secret env var, then asserts the audit log records the secret's
// source declaration but never its resolved value.
func TestAuditLogNeverContainsSecretValue(t *testing.T) {
	const secretValue = "super-secret-do-not-leak"
	t.Setenv("APP_SECRET", secretValue)
	sec, err := secrets.NewFromEnv("APP_SECRET")
	if err != nil {
		t.Fatal(err)
	}

	app := resource.Application{
		Metadata: resource.ApplicationMeta{Name: "api", Project: "beenaire", Environment: "staging"},
		Spec: resource.ApplicationSpec{
			BuildPack:   "dockerimage",
			Image:       &resource.ImageSpec{Name: "registry/api", Tag: "v1"},
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			Port:        8000,
			EnvVars:     []resource.EnvVarEntry{{Name: "TOKEN", ValueSecret: sec}},
		},
	}

	path := filepath.Join(t.TempDir(), "audit.log")
	resolved := state.Map{
		state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}: "proj-uuid",
		state.ResourceKey{Kind: state.KindServer, Name: "localhost"}:    "srv-uuid",
	}
	eng := apply.NewEngine(&mockClient{}, resolved, apply.NewAuditor(path))
	if _, aErr := eng.Apply(context.Background(), []apply.Operation{apply.ApplicationOp(apply.OpCreate, app, nil)}); aErr != nil {
		t.Fatal(aErr)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if strings.Contains(log, secretValue) {
		t.Fatal("audit log leaked the resolved secret value")
	}
	if !strings.Contains(log, "${env:APP_SECRET}") {
		t.Errorf("audit log should record the secret source declaration, got: %s", log)
	}
}
