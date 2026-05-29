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

// secretAppForAudit returns an application whose create exercises the enriched audit entry
// (a changed field for diff_hash and a secret env var for sources).
func secretAppForAudit(t *testing.T) (resource.Application, state.Map) {
	t.Helper()
	t.Setenv("AUDIT_SECRET", "do-not-leak")
	sec, err := secrets.NewFromEnv("AUDIT_SECRET")
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
	resolved := state.Map{
		state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}: "proj-uuid",
		state.ResourceKey{Kind: state.KindServer, Name: "localhost"}:    "srv-uuid",
	}
	return app, resolved
}

// recordAppCreate drives a create through the engine and returns the single decoded audit
// entry it wrote.
func recordAppCreate(t *testing.T, app resource.Application, resolved state.Map) apply.AuditEntry {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.log")
	eng := apply.NewEngine(&mockClient{}, resolved, apply.NewAuditor(path))
	if _, err := eng.Apply(context.Background(), []apply.Operation{apply.ApplicationOp(apply.OpCreate, app, nil)}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var e apply.AuditEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &e); err != nil {
		t.Fatalf("decode audit entry: %v (%s)", err, data)
	}
	return e
}

func TestAuditLogIncludesSources(t *testing.T) {
	app, resolved := secretAppForAudit(t)
	e := recordAppCreate(t, app, resolved)
	if len(e.Sources) != 1 || e.Sources[0] != "${env:AUDIT_SECRET}" {
		t.Errorf("Sources = %v, want [${env:AUDIT_SECRET}]", e.Sources)
	}
}

func TestAuditLogIncludesDiffHash(t *testing.T) {
	app, resolved := secretAppForAudit(t)
	e := recordAppCreate(t, app, resolved)
	if !strings.HasPrefix(e.DiffHash, "sha256:") {
		t.Errorf("DiffHash = %q, want a sha256: digest", e.DiffHash)
	}
}

func TestAuditLogIncludesActor(t *testing.T) {
	t.Setenv("IAC_COOLIFY_ACTOR", "ci-runner-7")
	app, resolved := secretAppForAudit(t)
	e := recordAppCreate(t, app, resolved)
	if e.Actor != "ci-runner-7" {
		t.Errorf("Actor = %q, want ci-runner-7", e.Actor)
	}
}

func TestAuditLogActorFallbackChain(t *testing.T) {
	tests := []struct {
		name      string
		explicit  string
		user      string
		wantActor string
	}{
		{name: "explicit wins", explicit: "deployer", user: "alice", wantActor: "deployer"},
		{name: "user fallback", explicit: "", user: "alice", wantActor: "alice"},
		{name: "unknown final", explicit: "", user: "", wantActor: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("IAC_COOLIFY_ACTOR", tt.explicit)
			t.Setenv("USER", tt.user)
			path := filepath.Join(t.TempDir(), "audit.log")
			if err := apply.NewAuditor(path).Record(apply.AuditEntry{Resource: "Project/p", Op: "create"}); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var e apply.AuditEntry
			if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &e); err != nil {
				t.Fatal(err)
			}
			if e.Actor != tt.wantActor {
				t.Errorf("Actor = %q, want %q", e.Actor, tt.wantActor)
			}
		})
	}
}
