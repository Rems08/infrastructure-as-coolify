package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// fakeClient serves canned live state to the importer, implementing importer.Client.
type fakeClient struct {
	servers      []coolify.Server
	resources    map[string][]coolify.ServerResource // server uuid → resources
	projects     []coolify.Project
	environments map[string][]coolify.Environment // project uuid → environments
	apps         map[string]coolify.Application
	dbs          map[string]coolify.Database
	appEnvs      map[string][]coolify.ServiceEnvVar
}

func (f *fakeClient) ListServers(context.Context) ([]coolify.Server, error) { return f.servers, nil }
func (f *fakeClient) GetServerResources(_ context.Context, uuid string) ([]coolify.ServerResource, error) {
	return f.resources[uuid], nil
}
func (f *fakeClient) ListProjects(context.Context) ([]coolify.Project, error) { return f.projects, nil }
func (f *fakeClient) ListEnvironments(_ context.Context, uuid string) ([]coolify.Environment, error) {
	return f.environments[uuid], nil
}
func (f *fakeClient) GetApplication(_ context.Context, uuid string) (coolify.Application, error) {
	return f.apps[uuid], nil
}
func (f *fakeClient) GetDatabase(_ context.Context, uuid string) (coolify.Database, error) {
	return f.dbs[uuid], nil
}
func (f *fakeClient) ListApplicationEnvs(_ context.Context, uuid string) ([]coolify.ServiceEnvVar, error) {
	return f.appEnvs[uuid], nil
}

// newFake builds a fixture: one server hosting a dockerimage app (staging), a git app
// (staging), a postgres database (staging), a service (skipped), and a production app.
func newFake() *fakeClient {
	return &fakeClient{
		servers: []coolify.Server{{UUID: "srv-1", Name: "localhost"}},
		resources: map[string][]coolify.ServerResource{
			"srv-1": {
				{UUID: "u-api", Name: "api", Type: "application"},
				{UUID: "u-worker", Name: "worker", Type: "application"},
				{UUID: "u-pg", Name: "pg-api-staging", Type: "standalone-postgresql"},
				{UUID: "u-svc", Name: "mail", Type: "service"},
				{UUID: "u-prod", Name: "api-prod", Type: "application"},
			},
		},
		projects: []coolify.Project{{UUID: "proj-bee", Name: "beenaire"}},
		environments: map[string][]coolify.Environment{
			"proj-bee": {{ID: 10, Name: "staging"}, {ID: 11, Name: "production"}},
		},
		apps: map[string]coolify.Application{
			"u-api": {Name: "api", FQDN: "https://api.example.com", BuildPack: "dockerimage",
				DockerRegistryImageName: "registry/api", DockerRegistryImageTag: "v1", PortsExposes: "8000", EnvironmentID: 10},
			"u-worker": {Name: "worker", BuildPack: "nixpacks", GitBranch: "main", PortsExposes: "3000", EnvironmentID: 10},
			"u-prod":   {Name: "api-prod", BuildPack: "dockerimage", DockerRegistryImageName: "registry/api", DockerRegistryImageTag: "v1", PortsExposes: "8000", EnvironmentID: 11},
		},
		dbs: map[string]coolify.Database{
			"u-pg": {Name: "pg-api-staging", DatabaseType: "standalone-postgresql", Image: "postgres:18-alpine",
				PostgresPassword: secrets.NewRemote("live-pg-password"), EnvironmentID: 10},
		},
		appEnvs: map[string][]coolify.ServiceEnvVar{
			"u-api": {{Key: "NODE_ENV", Value: "production"}, {Key: "DATABASE_URL", Value: "postgres://live:secret@host/db"}},
		},
	}
}

func runImport(t *testing.T, dir string, opts Options) Report {
	t.Helper()
	opts.Dir = dir
	rep, err := Run(context.Background(), newFake(), opts)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	return rep
}

func TestRun_ScaffoldsTree(t *testing.T) {
	dir := t.TempDir()
	rep := runImport(t, dir, Options{DefaultNetwork: "coolify", APIURL: "https://coolify.example.com"})

	for _, rel := range []string{
		"coolify.yaml",
		"environments/staging/applications/api.yaml",
		"environments/staging/applications/worker.yaml",
		"environments/staging/databases/pg-api-staging.yaml",
		"environments/production/applications/api-prod.yaml",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected scaffolded file %s: %v", rel, err)
		}
	}
	if len(rep.Applications) != 3 || len(rep.Databases) != 1 {
		t.Errorf("report counts: apps=%d dbs=%d, want 3 and 1", len(rep.Applications), len(rep.Databases))
	}
	if rep.completeCount() != 2 {
		t.Errorf("complete apps = %d, want 2 (api + api-prod; worker is partial)", rep.completeCount())
	}
	if len(rep.ServicesSkipped) != 1 || rep.ServicesSkipped[0] != "mail" {
		t.Errorf("services skipped = %v, want [mail]", rep.ServicesSkipped)
	}
	root, _ := os.ReadFile(filepath.Join(dir, "coolify.yaml"))
	if !strings.Contains(string(root), "api_url: https://coolify.example.com") {
		t.Errorf("root manifest missing api_url:\n%s", root)
	}
}

func TestRun_LeakSafe(t *testing.T) {
	dir := t.TempDir()
	rep := runImport(t, dir, Options{DefaultNetwork: "coolify"})

	apiYAML, _ := os.ReadFile(filepath.Join(dir, "environments/staging/applications/api.yaml"))
	for _, leak := range []string{"postgres://live:secret@host/db", "value: production"} {
		if strings.Contains(string(apiYAML), leak) {
			t.Errorf("application manifest leaked a clear env value %q:\n%s", leak, apiYAML)
		}
	}
	if !strings.Contains(string(apiYAML), "${env:NODE_ENV}") || !strings.Contains(string(apiYAML), "${env:DATABASE_URL}") {
		t.Errorf("env vars must be ${env:} references:\n%s", apiYAML)
	}

	pgYAML, _ := os.ReadFile(filepath.Join(dir, "environments/staging/databases/pg-api-staging.yaml"))
	if strings.Contains(string(pgYAML), "live-pg-password") {
		t.Errorf("database manifest leaked the live password:\n%s", pgYAML)
	}
	if !strings.Contains(string(pgYAML), "${env:PG_API_STAGING_PASSWORD}") {
		t.Errorf("database password must be a synthetic reference:\n%s", pgYAML)
	}

	wantKey := false
	for _, k := range rep.EnvKeys {
		if k == "DATABASE_URL" {
			wantKey = true
		}
	}
	if !wantKey {
		t.Errorf("report must list referenced env keys, got %v", rep.EnvKeys)
	}
	if len(rep.PasswordEnvs) != 1 || rep.PasswordEnvs[0] != "PG_API_STAGING_PASSWORD" {
		t.Errorf("report must list db password env, got %v", rep.PasswordEnvs)
	}
}

func TestRun_ConflictRefusesWithoutForce(t *testing.T) {
	dir := t.TempDir()
	collision := filepath.Join(dir, "environments/staging/applications/api.yaml")
	if err := os.MkdirAll(filepath.Dir(collision), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(collision, []byte("pre-existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), newFake(), Options{Dir: dir, DefaultNetwork: "coolify"})
	if err == nil {
		t.Fatal("import must refuse to overwrite an existing file without --force")
	}
	if !strings.Contains(err.Error(), "api.yaml") {
		t.Errorf("error must name the collision, got: %v", err)
	}
	// All-or-nothing: no other file was written.
	if _, sErr := os.Stat(filepath.Join(dir, "environments/staging/databases/pg-api-staging.yaml")); !os.IsNotExist(sErr) {
		t.Error("a refused import must write nothing")
	}
	if got, _ := os.ReadFile(collision); string(got) != "pre-existing\n" {
		t.Error("a refused import must not touch the existing file")
	}
}

func TestRun_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	collision := filepath.Join(dir, "environments/staging/applications/api.yaml")
	if err := os.MkdirAll(filepath.Dir(collision), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(collision, []byte("pre-existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runImport(t, dir, Options{DefaultNetwork: "coolify", Force: true})
	if got, _ := os.ReadFile(collision); string(got) == "pre-existing\n" {
		t.Error("--force must overwrite the existing file")
	}
}

func TestRun_EnvFilter(t *testing.T) {
	dir := t.TempDir()
	rep := runImport(t, dir, Options{DefaultNetwork: "coolify", EnvFilter: []string{"staging"}})

	if _, err := os.Stat(filepath.Join(dir, "environments/production/applications/api-prod.yaml")); !os.IsNotExist(err) {
		t.Error("--env staging must not write production resources")
	}
	if _, err := os.Stat(filepath.Join(dir, "environments/staging/applications/api.yaml")); err != nil {
		t.Errorf("--env staging must still write staging resources: %v", err)
	}
	for _, a := range rep.Applications {
		if a.Environment == "production" {
			t.Errorf("report must exclude production app under --env staging: %+v", a)
		}
	}
}

func TestRun_DroppedWhenEnvUnknown(t *testing.T) {
	dir := t.TempDir()
	fake := newFake()
	// An application whose environment_id was never enumerated must be counted, not silent.
	fake.apps["u-api"] = coolify.Application{Name: "api", BuildPack: "dockerimage",
		DockerRegistryImageName: "registry/api", DockerRegistryImageTag: "v1", PortsExposes: "8000", EnvironmentID: 999}

	rep, err := Run(context.Background(), fake, Options{Dir: dir, DefaultNetwork: "coolify"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Dropped != 1 {
		t.Errorf("dropped = %d, want 1", rep.Dropped)
	}
}

// TestRun_PrefersLiveDestinationNetwork asserts the network is taken from the live
// destination payload when present, so the imported manifest matches what plan compares
// against; DefaultNetwork only fills the gap for payloads without one.
func TestRun_PrefersLiveDestinationNetwork(t *testing.T) {
	dir := t.TempDir()
	fake := newFake()
	app := fake.apps["u-api"]
	app.Destination = coolify.Destination{Network: "live-net", Server: coolify.Server{UUID: "srv-1", Name: "localhost"}}
	fake.apps["u-api"] = app
	db := fake.dbs["u-pg"]
	db.Destination = coolify.Destination{Network: "live-net", Server: coolify.Server{UUID: "srv-1", Name: "localhost"}}
	fake.dbs["u-pg"] = db

	if _, err := Run(context.Background(), fake, Options{Dir: dir, DefaultNetwork: "fallback-net"}); err != nil {
		t.Fatalf("import: %v", err)
	}
	for file, wantNet := range map[string]string{
		"environments/staging/applications/api.yaml":         "live-net",
		"environments/staging/databases/pg-api-staging.yaml": "live-net",
		"environments/staging/applications/worker.yaml":      "fallback-net",
	} {
		body, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "network: "+wantNet) {
			t.Errorf("%s: want network %q, got:\n%s", file, wantNet, body)
		}
	}
}
