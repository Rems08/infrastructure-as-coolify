package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// importMux serves the documented read endpoints the import path uses: per-server
// enumeration, project/environment listing, and per-resource detail.
func importMux(t *testing.T) *httptest.Server {
	t.Helper()
	bodies := map[string]string{
		"/api/v1/projects":                       `[{"id":1,"uuid":"proj-bee","name":"beenaire"}]`,
		"/api/v1/projects/proj-bee/environments": `[{"id":10,"name":"staging"}]`,
		"/api/v1/servers":                        `[{"uuid":"srv-1","name":"localhost"}]`,
		"/api/v1/servers/srv-1/resources":        `[{"uuid":"u-api","name":"api","type":"application"},{"uuid":"u-pg","name":"pg-api-staging","type":"standalone-postgresql"}]`,
		"/api/v1/applications/u-api":             `{"uuid":"u-api","name":"api","fqdn":"https://api.example.com","build_pack":"dockerimage","docker_registry_image_name":"registry/api","docker_registry_image_tag":"v1","ports_exposes":"8000","environment_id":10}`,
		"/api/v1/applications/u-api/envs":        `[{"key":"NODE_ENV","value":"production"}]`,
		"/api/v1/databases/u-pg":                 `{"uuid":"u-pg","name":"pg-api-staging","database_type":"standalone-postgresql","image":"postgres:18-alpine","postgres_password":"live-pg-password","environment_id":10}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := bodies[r.URL.Path]
		if !ok {
			t.Errorf("unexpected path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestImportCommand_ScaffoldsAndReports(t *testing.T) {
	clearCoolifyEnv(t)
	srv := importMux(t)
	t.Setenv("COOLIFY_API_TOKEN", "tok_import_DO_NOT_LEAK")

	dir := t.TempDir()
	out, err := runCmd(t, "import", dir, "--coolify-url="+srv.URL)
	if err != nil {
		t.Fatalf("import: %v (out: %s)", err, out)
	}

	for _, rel := range []string{
		"coolify.yaml",
		"environments/staging/applications/api.yaml",
		"environments/staging/databases/pg-api-staging.yaml",
	} {
		if _, sErr := os.Stat(filepath.Join(dir, rel)); sErr != nil {
			t.Errorf("expected scaffolded file %s: %v", rel, sErr)
		}
	}
	if !strings.Contains(out, "imported 1 application(s) (1 complete, 0 partial), 1 database(s)") {
		t.Errorf("report summary missing:\n%s", out)
	}

	apiYAML, _ := os.ReadFile(filepath.Join(dir, "environments/staging/applications/api.yaml"))
	if strings.Contains(string(apiYAML), "value: production") {
		t.Errorf("clear env value written:\n%s", apiYAML)
	}
	if !strings.Contains(string(apiYAML), "${env:NODE_ENV}") {
		t.Errorf("env var reference missing:\n%s", apiYAML)
	}
	pgYAML, _ := os.ReadFile(filepath.Join(dir, "environments/staging/databases/pg-api-staging.yaml"))
	if strings.Contains(string(pgYAML), "live-pg-password") {
		t.Errorf("database manifest leaked the live password:\n%s", pgYAML)
	}
}

func TestImportCommand_RequiresCredentials(t *testing.T) {
	clearCoolifyEnv(t)
	if _, err := runCmd(t, "import", t.TempDir()); err == nil {
		t.Fatal("import must require Coolify credentials")
	}
}
