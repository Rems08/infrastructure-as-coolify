package apply_test

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

const canaryCompose = "version: '3'\nservices:\n  db:\n    image: postgres:16\n    environment:\n      DB_PASSWORD: super-secret-123\n"

func serviceResolved() state.Map {
	return state.Map{
		state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}:                            "proj-uuid",
		state.ResourceKey{Kind: state.KindServer, Name: "localhost"}:                               "srv-uuid",
		state.ResourceKey{Project: "beenaire", Kind: resource.KindEnvironment, Name: "production"}: "production",
	}
}

func composeService(envs []resource.EnvVarEntry) resource.Service {
	return resource.Service{
		Metadata: resource.ServiceMeta{Name: "observability", Project: "beenaire", Environment: "production"},
		Spec: resource.ServiceSpec{
			Destination:       resource.ServiceDestination{Server: "localhost"},
			DockerComposePath: "docker-compose.yml",
			EnvVars:           envs,
		},
	}
}

func TestServiceE2ECreateFromComposePath(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	eng := apply.NewEngine(e2eClient(t, srv.URL), serviceResolved(), nil)

	op := apply.ServiceOp(apply.OpCreate, composeService(nil), canaryCompose, nil)
	if _, err := eng.Apply(context.Background(), []apply.Operation{op}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := paths(*reqs); len(got) != 1 || got[0] != "POST /api/v1/services" {
		t.Fatalf("requests = %v, want one POST /services", got)
	}
	encoded, _ := (*reqs)[0].body["docker_compose_raw"].(string)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(decoded) != canaryCompose {
		t.Errorf("docker_compose_raw not a valid base64 round-trip of the compose file: %v", err)
	}
}

func TestServiceE2ECreateFromType(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	eng := apply.NewEngine(e2eClient(t, srv.URL), serviceResolved(), nil)

	svc := resource.Service{
		Metadata: resource.ServiceMeta{Name: "gitea", Project: "beenaire", Environment: "production"},
		Spec: resource.ServiceSpec{
			Destination: resource.ServiceDestination{Server: "localhost"},
			Type:        "gitea-with-mysql",
		},
	}
	op := apply.ServiceOp(apply.OpCreate, svc, "", nil)
	if _, err := eng.Apply(context.Background(), []apply.Operation{op}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	body := (*reqs)[0].body
	if body["type"] != "gitea-with-mysql" {
		t.Errorf("type passthrough missing: %+v", body)
	}
	if _, present := body["docker_compose_raw"]; present {
		t.Errorf("one-click type must not send docker_compose_raw: %+v", body)
	}
}

func TestServiceE2EBulkEnvUpdate(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	t.Setenv("GRAFANA_PASS", "s3cr3t-grafana")
	sec, err := secrets.NewFromEnv("GRAFANA_PASS")
	if err != nil {
		t.Fatal(err)
	}
	envs := []resource.EnvVarEntry{
		{Name: "GRAFANA_USER", Value: "admin"},
		{Name: "GRAFANA_PASSWORD", ValueSecret: sec},
	}
	eng := apply.NewEngine(e2eClient(t, srv.URL), serviceResolved(), nil)
	op := apply.ServiceOp(apply.OpCreate, composeService(envs), canaryCompose, nil)
	if _, err := eng.Apply(context.Background(), []apply.Operation{op}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := []string{"POST /api/v1/services", "PATCH /api/v1/services/app-uuid/envs/bulk"}
	got := paths(*reqs)
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	data, _ := (*reqs)[1].body["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("bulk data = %+v, want 2 entries", (*reqs)[1].body["data"])
	}
	second, _ := data[1].(map[string]any)
	if second["value"] != "s3cr3t-grafana" {
		t.Errorf("secret env not revealed at the boundary for deployment: %+v", second)
	}
}

func TestServiceE2EDeleteReverseOrder(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	resolved := serviceResolved()
	resolved[state.ResourceKey{Project: "beenaire", Environment: "production", Kind: resource.KindService, Name: "observability"}] = "svc-uuid"
	ops, err := apply.OrderDelete([]apply.Operation{
		{Op: apply.OpDelete, Kind: resource.KindProject, Name: "beenaire"},
		{Op: apply.OpDelete, Kind: resource.KindEnvironment, Project: "beenaire", Name: "production"},
		{Op: apply.OpDelete, Kind: resource.KindService, Project: "beenaire", Environment: "production", Name: "observability"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apply.NewEngine(e2eClient(t, srv.URL), resolved, nil).Apply(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DELETE /api/v1/services/svc-uuid",
		"DELETE /api/v1/projects/proj-uuid/environments/production",
		"DELETE /api/v1/projects/proj-uuid",
	}
	got := paths(*reqs)
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("delete order = %v, want %v", got, want)
		}
	}
}

func TestServiceE2ELifecycle(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	c := e2eClient(t, srv.URL)
	ctx := context.Background()
	if _, err := c.CreateService(ctx, coolify.CreateServiceRequest{
		Type: "gitea-with-mysql", ProjectUUID: "proj-uuid", ServerUUID: "srv-uuid",
	}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []func(context.Context, string) error{c.StartService, c.StopService, c.RestartService} {
		if err := step(ctx, "svc-1"); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.DeleteService(ctx, "svc-1"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"POST /api/v1/services",
		"POST /api/v1/services/svc-1/start",
		"POST /api/v1/services/svc-1/stop",
		"POST /api/v1/services/svc-1/restart",
		"DELETE /api/v1/services/svc-1",
	}
	got := paths(*reqs)
	if len(got) != len(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request[%d] = %q, want %q", i, got[i], want[i])
		}
		if (*reqs)[i].idemp == "" {
			t.Errorf("%s missing Idempotency-Key", got[i])
		}
	}
}

func TestAuditLogServiceNeverContainsComposeContent(t *testing.T) {
	srv, _ := e2eServer(t, "")
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	t.Setenv("GRAFANA_PASS", "super-secret-123")
	sec, err := secrets.NewFromEnv("GRAFANA_PASS")
	if err != nil {
		t.Fatal(err)
	}
	eng := apply.NewEngine(e2eClient(t, srv.URL), serviceResolved(), apply.NewAuditor(auditPath))
	op := apply.ServiceOp(apply.OpCreate, composeService([]resource.EnvVarEntry{
		{Name: "GRAFANA_PASSWORD", ValueSecret: sec},
	}), canaryCompose, nil)
	if _, aErr := eng.Apply(context.Background(), []apply.Operation{op}); aErr != nil {
		t.Fatal(aErr)
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	canaries := []string{
		"version:", "services:", "environment:", "super-secret-123",
		"postgres:16", base64.StdEncoding.EncodeToString([]byte(canaryCompose)),
	}
	for _, c := range canaries {
		if strings.Contains(log, c) {
			t.Errorf("audit log leaked compose content or secret: %q present", c)
		}
	}
	if !strings.Contains(log, `"compose_hash":"sha256:`) {
		t.Errorf("audit log missing compose_hash: %s", log)
	}
	// The secret's source declaration is recorded; its value is not.
	if !strings.Contains(log, "${env:GRAFANA_PASS}") {
		t.Errorf("audit log missing the secret source declaration: %s", log)
	}
}

func TestAuditLogServicePermissions0600(t *testing.T) {
	srv, _ := e2eServer(t, "")
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	eng := apply.NewEngine(e2eClient(t, srv.URL), serviceResolved(), apply.NewAuditor(auditPath))
	op := apply.ServiceOp(apply.OpCreate, composeService(nil), canaryCompose, nil)
	if _, err := eng.Apply(context.Background(), []apply.Operation{op}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("audit log mode = %o, want 600", perm)
	}
}
