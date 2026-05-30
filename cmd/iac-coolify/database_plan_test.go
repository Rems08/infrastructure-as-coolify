package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dbManifest is one Database YAML document for the Beenaire fixture.
func dbManifest(name, project, env, engine, image string) string {
	return "api_version: iac-coolify/v1\n" +
		"kind: Database\n" +
		"metadata:\n" +
		"  name: " + name + "\n" +
		"  project: " + project + "\n" +
		"  environment: " + env + "\n" +
		"spec:\n" +
		"  engine: " + engine + "\n" +
		"  image: " + image + "\n" +
		"  destination:\n" +
		"    server: localhost\n" +
		"    network: coolify\n"
}

// beenaireDBDir writes the three real Beenaire databases as YAML manifests.
func beenaireDBDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	manifests := map[string]string{
		"pg.yaml":            dbManifest("pg-restaurant-core-api-staging", "beenaire", "staging", "postgresql", "postgres:18-alpine"),
		"redis-staging.yaml": dbManifest("redis-database-restaurant-core-api-staging", "beenaire", "staging", "redis", "redis:7-alpine"),
		"redis-prod.yaml":    dbManifest("redis-database-restaurant-core-api", "beenaire", "production", "redis", "redis:7-alpine"),
	}
	for file, body := range manifests {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const (
	pgSecret    = "PG-PASSWORD-DO-NOT-LEAK"
	redisSecret = "REDIS-PASSWORD-DO-NOT-LEAK"
)

// coolifyDBMux serves the documented endpoints the plan path uses plus a typed
// GET /databases/{uuid} per database (whose response schema is a placeholder in the spec).
// The database bodies are configured so a stable Beenaire matches its manifest exactly.
func coolifyDBMux(t *testing.T) *httptest.Server {
	t.Helper()
	resources := `[` +
		`{"id":1,"uuid":"db-pg","name":"pg-restaurant-core-api-staging","type":"standalone-postgresql","status":"running:healthy","created_at":"","updated_at":""},` +
		`{"id":2,"uuid":"db-redis-staging","name":"redis-database-restaurant-core-api-staging","type":"standalone-redis","status":"running:healthy","created_at":"","updated_at":""},` +
		`{"id":3,"uuid":"db-redis-prod","name":"redis-database-restaurant-core-api","type":"standalone-redis","status":"running:healthy","created_at":"","updated_at":""}` +
		`]`
	pg := `{"uuid":"db-pg","name":"pg-restaurant-core-api-staging","image":"postgres:18-alpine",` +
		`"is_public":false,"public_port":5432,"limits_cpu_shares":1024,"limits_memory":"0",` +
		`"postgres_password":"` + pgSecret + `","internal_db_url":"postgres://u:` + pgSecret + `@host/db","status":"running:healthy"}`
	redisStaging := `{"uuid":"db-redis-staging","name":"redis-database-restaurant-core-api-staging","image":"redis:7-alpine",` +
		`"is_public":false,"public_port":6379,"limits_cpu_shares":1024,"limits_memory":"0",` +
		`"redis_password":"` + redisSecret + `","status":"running:healthy"}`
	redisProd := `{"uuid":"db-redis-prod","name":"redis-database-restaurant-core-api","image":"redis:7-alpine",` +
		`"is_public":false,"public_port":6379,"limits_cpu_shares":1024,"limits_memory":"0",` +
		`"redis_password":"` + redisSecret + `","status":"running:healthy"}`
	bodies := map[string]string{
		"/api/v1/projects":                   `[{"id":1,"uuid":"p1","name":"beenaire"}]`,
		"/api/v1/projects/p1/environments":   `[{"id":10,"name":"staging"},{"id":11,"name":"production"}]`,
		"/api/v1/servers":                    `[{"uuid":"srv-1","name":"localhost"}]`,
		"/api/v1/applications":               `[]`,
		"/api/v1/services":                   `[]`,
		"/api/v1/servers/srv-1/resources":    resources,
		"/api/v1/databases/db-pg":            pg,
		"/api/v1/databases/db-redis-staging": redisStaging,
		"/api/v1/databases/db-redis-prod":    redisProd,
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

func TestApply_DatabaseZeroChange_BeenaireFixture(t *testing.T) {
	clearCoolifyEnv(t)
	srv := coolifyDBMux(t)
	t.Setenv("COOLIFY_API_TOKEN", "tok_DO_NOT_LEAK")
	out, err := runCmd(t, "plan", beenaireDBDir(t),
		"--coolify-url", srv.URL, "--output=text", "--detailed-exitcode",
		"--openapi-dir", filepath.Join("..", "..", "testdata", "openapi"))
	if err != nil {
		t.Fatalf("stable Beenaire databases must plan with no changes (exit 0), got %v\n%s", err, out)
	}
	if !strings.Contains(out, "0 to add, 0 to change, 0 to destroy") {
		t.Errorf("expected zero-change plan, got:\n%s", out)
	}
	for _, name := range []string{
		"pg-restaurant-core-api-staging",
		"redis-database-restaurant-core-api-staging",
		"redis-database-restaurant-core-api",
	} {
		if !strings.Contains(out, "Database."+name+": no changes") {
			t.Errorf("database %q missing from plan:\n%s", name, out)
		}
	}
}

func TestNoCredentialLeak_DatabasePlanOutput(t *testing.T) {
	clearCoolifyEnv(t)
	srv := coolifyDBMux(t)
	t.Setenv("COOLIFY_API_TOKEN", "tok_DO_NOT_LEAK")
	for _, format := range []string{"text", "json"} {
		out, err := runCmd(t, "plan", beenaireDBDir(t),
			"--coolify-url", srv.URL, "--output="+format,
			"--openapi-dir", filepath.Join("..", "..", "testdata", "openapi"))
		if err != nil {
			t.Fatalf("%s plan: %v\n%s", format, err, out)
		}
		for _, leak := range []string{pgSecret, redisSecret, "tok_DO_NOT_LEAK"} {
			if strings.Contains(out, leak) {
				t.Errorf("%s plan output leaked credential %q:\n%s", format, leak, out)
			}
		}
	}
}
