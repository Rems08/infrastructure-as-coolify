package state_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

func newClient(t *testing.T, baseURL string) *coolify.Client {
	t.Helper()
	t.Setenv("COOLIFY_API_TOKEN", "tok_resolver_DO_NOT_LEAK")
	tok, err := secrets.NewFromEnv("COOLIFY_API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	c, err := coolify.NewClient(coolify.Options{BaseURL: baseURL, Token: tok})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func serve(t *testing.T, bodies map[string]string, status map[string]int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code, ok := status[r.URL.Path]; ok {
			w.WriteHeader(code)
			return
		}
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

func key(project, env, name string) state.ResourceKey {
	return state.ResourceKey{Project: project, Environment: env, Kind: resource.KindApplication, Name: name}
}

func TestResolveUUIDsFromHTTPTest(t *testing.T) {
	// Two projects, multiple environments, multiple applications.
	bodies := map[string]string{
		"/api/v1/projects": `[
			{"id":1,"uuid":"proj-bee","name":"beenaire"},
			{"id":2,"uuid":"proj-lab","name":"labs"}
		]`,
		"/api/v1/projects/proj-bee/environments": `[
			{"id":10,"name":"staging"},
			{"id":11,"name":"production"}
		]`,
		"/api/v1/projects/proj-lab/environments": `[
			{"id":20,"name":"staging"}
		]`,
		"/api/v1/servers": `[
			{"uuid":"srv-1","name":"localhost"},
			{"uuid":"srv-2","name":"edge"}
		]`,
		"/api/v1/applications": `[
			{"uuid":"uuid-web-stg","name":"web","environment_id":10},
			{"uuid":"uuid-web-prod","name":"web","environment_id":11},
			{"uuid":"uuid-api-stg","name":"api","environment_id":10},
			{"uuid":"uuid-lab-x","name":"x","environment_id":20},
			{"uuid":"uuid-orphan","name":"orphan","environment_id":999}
		]`,
		"/api/v1/services": `[
			{"uuid":"uuid-obs-prod","name":"observability","environment_id":11},
			{"uuid":"uuid-obs-stg","name":"observability","environment_id":10},
			{"uuid":"uuid-svc-orphan","name":"orphan-svc","environment_id":999}
		]`,
		"/api/v1/servers/srv-1/resources": `[]`,
		"/api/v1/servers/srv-2/resources": `[]`,
	}
	srv := serve(t, bodies, nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	pKey := func(name string) state.ResourceKey {
		return state.ResourceKey{Kind: resource.KindProject, Name: name}
	}
	eKey := func(project, name string) state.ResourceKey {
		return state.ResourceKey{Project: project, Kind: resource.KindEnvironment, Name: name}
	}
	sKey := func(name string) state.ResourceKey {
		return state.ResourceKey{Kind: state.KindServer, Name: name}
	}
	svcKey := func(project, env, name string) state.ResourceKey {
		return state.ResourceKey{Project: project, Environment: env, Kind: resource.KindService, Name: name}
	}
	want := map[state.ResourceKey]string{
		// Applications: keyed (project, env, name) → uuid.
		key("beenaire", "staging", "web"):    "uuid-web-stg",  // found
		key("beenaire", "production", "web"): "uuid-web-prod", // multi-env, same name
		key("beenaire", "staging", "api"):    "uuid-api-stg",
		key("labs", "staging", "x"):          "uuid-lab-x", // multi-project
		// Projects → uuid.
		pKey("beenaire"): "proj-bee",
		pKey("labs"):     "proj-lab",
		// Environments → name (no UUID in the schema).
		eKey("beenaire", "staging"):    "staging",
		eKey("beenaire", "production"): "production",
		eKey("labs", "staging"):        "staging",
		// Servers → uuid.
		sKey("localhost"): "srv-1",
		sKey("edge"):      "srv-2",
		// Services: keyed (project, env, name) → uuid; same name across envs.
		svcKey("beenaire", "production", "observability"): "uuid-obs-prod",
		svcKey("beenaire", "staging", "observability"):    "uuid-obs-stg",
	}
	if len(m) != len(want) {
		t.Fatalf("map size = %d, want %d (orphan must be skipped): %+v", len(m), len(want), m)
	}
	for k, wantID := range want {
		got, ok := m.Lookup(k)
		if !ok || got != wantID {
			t.Errorf("Lookup(%v) = %q,%v want %q", k, got, ok, wantID)
		}
	}

	// missing: an app in an unlisted environment is skipped, not mis-keyed.
	if _, ok := m.Lookup(key("beenaire", "staging", "orphan")); ok {
		t.Error("orphan app (unknown environment_id) must not be resolved")
	}
	// missing: a key that was never declared.
	if _, ok := m.Lookup(key("beenaire", "staging", "ghost")); ok {
		t.Error("ghost key must not resolve")
	}
}

// TestResolve_AppsAndServicesResolvedWithoutProjectID reproduces the live API shape:
// GET /projects/{uuid}/environments returns {id, uuid, name} with no project_id. The
// resolver must still scope every application and service under its project (derived while
// enumerating environments), across more than one project. On code that derived the project
// from env.project_id this fails — every child is dropped and the map holds only the
// project and environment keys.
func TestResolve_AppsAndServicesResolvedWithoutProjectID(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects": `[
			{"id":1,"uuid":"proj-bee","name":"beenaire"},
			{"id":2,"uuid":"proj-lab","name":"labs"}
		]`,
		"/api/v1/projects/proj-bee/environments": `[
			{"id":1,"name":"production"},
			{"id":4,"name":"staging"}
		]`,
		"/api/v1/projects/proj-lab/environments": `[
			{"id":7,"name":"staging"}
		]`,
		"/api/v1/servers": `[]`,
		"/api/v1/applications": `[
			{"uuid":"u-bo-prod","name":"back-office","environment_id":1},
			{"uuid":"u-bo-stg","name":"back-office","environment_id":4},
			{"uuid":"u-lab","name":"sandbox","environment_id":7}
		]`,
		"/api/v1/services": `[
			{"uuid":"u-obs","name":"observability","environment_id":1}
		]`,
	}
	srv := serve(t, bodies, nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}

	svcKey := func(project, env, name string) state.ResourceKey {
		return state.ResourceKey{Project: project, Environment: env, Kind: resource.KindService, Name: name}
	}
	wantApps := map[state.ResourceKey]string{
		key("beenaire", "production", "back-office"): "u-bo-prod",
		key("beenaire", "staging", "back-office"):    "u-bo-stg", // same name, sibling env
		key("labs", "staging", "sandbox"):            "u-lab",    // a second project, distinct env id
	}
	for k, want := range wantApps {
		if got, ok := m.Lookup(k); !ok || got != want {
			t.Errorf("Lookup(%v) = %q,%v want %q (app dropped — project not derived from enumeration)", k, got, ok, want)
		}
	}
	if got, ok := m.Lookup(svcKey("beenaire", "production", "observability")); !ok || got != "u-obs" {
		t.Errorf("service observability = %q,%v want u-obs", got, ok)
	}
}

// TestResolve_WarnsAndCountsUnmappedChildren asserts an application whose environment_id was
// never enumerated is logged and counted, not dropped in silence.
func TestResolve_WarnsAndCountsUnmappedChildren(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	bodies := map[string]string{
		"/api/v1/projects":                       `[{"id":1,"uuid":"proj-bee","name":"beenaire"}]`,
		"/api/v1/projects/proj-bee/environments": `[{"id":10,"name":"staging"}]`,
		"/api/v1/servers":                        `[]`,
		"/api/v1/applications": `[
			{"uuid":"u-ok","name":"web","environment_id":10},
			{"uuid":"u-orphan","name":"ghost","environment_id":999}
		]`,
		"/api/v1/services": `[]`,
	}
	srv := serve(t, bodies, nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := m.Lookup(key("beenaire", "staging", "ghost")); ok {
		t.Error("app with unknown environment_id must not be keyed")
	}

	rec := findLogRecord(t, buf.Bytes(), "resolved applications and services")
	if got := rec["resolver.children.resolved"]; got != float64(1) {
		t.Errorf("resolved count = %v, want 1", got)
	}
	if got := rec["resolver.children.dropped"]; got != float64(1) {
		t.Errorf("dropped count = %v, want 1", got)
	}
	warn := findLogRecord(t, buf.Bytes(), "resolve: resource skipped, environment_id not found")
	if got := warn["resolver.child.name"]; got != "ghost" {
		t.Errorf("warn names %v, want ghost", got)
	}
	if got := warn["resolver.child.environment_id"]; got != float64(999) {
		t.Errorf("warn environment_id = %v, want 999", got)
	}
}

func TestResolveEmpty(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects":     `[]`,
		"/api/v1/servers":      `[]`,
		"/api/v1/applications": `[]`,
		"/api/v1/services":     `[]`,
	}
	srv := serve(t, bodies, nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Errorf("empty instance must resolve to 0 entries, got %d", len(m))
	}
}

func TestResolveProjectsError(t *testing.T) {
	srv := serve(t, map[string]string{}, map[string]int{"/api/v1/projects": http.StatusNotFound})
	if _, err := state.Resolve(context.Background(), newClient(t, srv.URL)); err == nil {
		t.Fatal("want error when /projects fails")
	}
}

func TestResolveEnvironmentsError(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects": `[{"id":1,"uuid":"proj-bee","name":"beenaire"}]`,
	}
	srv := serve(t, bodies, map[string]int{"/api/v1/projects/proj-bee/environments": http.StatusBadRequest})
	if _, err := state.Resolve(context.Background(), newClient(t, srv.URL)); err == nil {
		t.Fatal("want error when /environments fails")
	}
}

func TestResolveServersError(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects":                       `[{"id":1,"uuid":"proj-bee","name":"beenaire"}]`,
		"/api/v1/projects/proj-bee/environments": `[]`,
	}
	srv := serve(t, bodies, map[string]int{"/api/v1/servers": http.StatusInternalServerError})
	if _, err := state.Resolve(context.Background(), newClient(t, srv.URL)); err == nil {
		t.Fatal("want error when /servers fails")
	}
}

func TestResolveServicesError(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects":                       `[{"id":1,"uuid":"proj-bee","name":"beenaire"}]`,
		"/api/v1/projects/proj-bee/environments": `[]`,
		"/api/v1/servers":                        `[]`,
		"/api/v1/applications":                   `[]`,
	}
	srv := serve(t, bodies, map[string]int{"/api/v1/services": http.StatusBadRequest})
	if _, err := state.Resolve(context.Background(), newClient(t, srv.URL)); err == nil {
		t.Fatal("want error when /services fails")
	}
}

func dbKey(name string) state.ResourceKey {
	return state.ResourceKey{Kind: resource.KindDatabase, Name: name}
}

func serversResourcesFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "servers-resources.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// oneServerBodies returns a minimal Resolve body set with a single server whose
// /resources endpoint serves resourcesBody.
func oneServerBodies(resourcesBody string) map[string]string {
	return map[string]string{
		"/api/v1/projects":                  `[]`,
		"/api/v1/servers":                   `[{"uuid":"srv-bee","name":"localhost"}]`,
		"/api/v1/applications":              `[]`,
		"/api/v1/services":                  `[]`,
		"/api/v1/servers/srv-bee/resources": resourcesBody,
	}
}

func TestResolveDatabases_filtersStandalonePrefix(t *testing.T) {
	srv := serve(t, oneServerBodies(serversResourcesFixture(t)), nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"pg-restaurant-core-api":                     "dpzndbf9fgt7uqyoucwt8986",
		"pg-restaurant-core-api-staging":             "t50fefd4yb1salodq9bipiw3",
		"redis-database-restaurant-core-api":         "oqsk32s77rp7svh6r8cgmkw9",
		"redis-database-restaurant-core-api-staging": "dj50mi6c35is4cjh7x52ww43",
	}
	var dbCount int
	for k := range m {
		if k.Kind == resource.KindDatabase {
			dbCount++
		}
	}
	if dbCount != len(want) {
		t.Fatalf("resolved %d databases, want %d: %+v", dbCount, len(want), m)
	}
	for name, uuid := range want {
		got, ok := m.Lookup(dbKey(name))
		if !ok || got != uuid {
			t.Errorf("Lookup(db %q) = %q,%v want %q", name, got, ok, uuid)
		}
	}
}

func TestResolveDatabases_skipsApplicationAndService(t *testing.T) {
	srv := serve(t, oneServerBodies(serversResourcesFixture(t)), nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Beenaire DOC", "restaurant-core-api-staging", "kafka-restaurant-core-api", "restaurant-core-api-workers"} {
		if _, ok := m.Lookup(dbKey(name)); ok {
			t.Errorf("non-standalone resource %q must not be keyed as a Database", name)
		}
	}
}

func TestResolveDatabases_multipleServers(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects":     `[]`,
		"/api/v1/servers":      `[{"uuid":"srv-a","name":"a"},{"uuid":"srv-b","name":"b"}]`,
		"/api/v1/applications": `[]`,
		"/api/v1/services":     `[]`,
		"/api/v1/servers/srv-a/resources": `[
			{"id":1,"uuid":"u-pg","name":"pg-main","type":"standalone-postgresql","status":"running:healthy"},
			{"id":2,"uuid":"u-app","name":"web","type":"application","status":"running:healthy"}
		]`,
		"/api/v1/servers/srv-b/resources": `[
			{"id":3,"uuid":"u-redis","name":"redis-main","type":"standalone-redis","status":"running:healthy"}
		]`,
	}
	srv := serve(t, bodies, nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := m.Lookup(dbKey("pg-main")); !ok || got != "u-pg" {
		t.Errorf("pg-main = %q,%v want u-pg", got, ok)
	}
	if got, ok := m.Lookup(dbKey("redis-main")); !ok || got != "u-redis" {
		t.Errorf("redis-main = %q,%v want u-redis", got, ok)
	}
}

func TestResolveDatabases_emptyServerList(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects":     `[]`,
		"/api/v1/servers":      `[]`,
		"/api/v1/applications": `[]`,
		"/api/v1/services":     `[]`,
	}
	srv := serve(t, bodies, nil)
	m, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err != nil {
		t.Fatalf("empty server list must be a no-op, got error: %v", err)
	}
	for k := range m {
		if k.Kind == resource.KindDatabase {
			t.Errorf("no databases expected with zero servers, got %v", k)
		}
	}
}

func TestResolveDatabases_propagatesResourcesError(t *testing.T) {
	bodies := oneServerBodies("")
	delete(bodies, "/api/v1/servers/srv-bee/resources")
	srv := serve(t, bodies, map[string]int{"/api/v1/servers/srv-bee/resources": http.StatusInternalServerError})
	_, err := state.Resolve(context.Background(), newClient(t, srv.URL))
	if err == nil {
		t.Fatal("want error when /servers/{uuid}/resources fails")
	}
	if !strings.Contains(err.Error(), "srv-bee") {
		t.Errorf("error %q must name the failing server for debugging", err)
	}
}

func TestResolveDatabases_logsCounters(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	srv := serve(t, oneServerBodies(serversResourcesFixture(t)), nil)
	if _, err := state.Resolve(context.Background(), newClient(t, srv.URL)); err != nil {
		t.Fatal(err)
	}

	rec := findLogRecord(t, buf.Bytes(), "resolved databases")
	wantAttrs := map[string]float64{
		"resolver.databases.servers_scanned":     1,
		"resolver.databases.resources_total":     9,
		"resolver.databases.standalone_filtered": 4,
		"resolver.databases.resolved":            4,
	}
	for key, want := range wantAttrs {
		got, ok := rec[key].(float64)
		if !ok || got != want {
			t.Errorf("attr %q = %v (ok=%v), want %v", key, rec[key], ok, want)
		}
	}
}

// findLogRecord returns the first JSON slog record whose msg matches.
func findLogRecord(t *testing.T, out []byte, msg string) map[string]any {
	t.Helper()
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal(line, &rec); err != nil {
			t.Fatalf("log line is not JSON: %s", line)
		}
		if rec["msg"] == msg {
			return rec
		}
	}
	t.Fatalf("no log record with msg %q in:\n%s", msg, out)
	return nil
}

func TestMapSaveRoundTrip(t *testing.T) {
	m := state.Map{
		key("beenaire", "staging", "web"): "uuid-web-stg",
		key("labs", "staging", "x"):       "uuid-lab-x",
	}
	path := filepath.Join(t.TempDir(), ".iac-coolify", "state.json")
	at := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	if err := m.Save(path, "deadbeef", at); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		UUIDs       map[string]string `json:"uuids"`
		OpenAPIHash string            `json:"openapi_hash"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if st.UUIDs["beenaire/staging/Application/web"] != "uuid-web-stg" {
		t.Errorf("cache missing expected entry: %v", st.UUIDs)
	}
	if st.OpenAPIHash != "deadbeef" {
		t.Errorf("openapi_hash = %q, want deadbeef", st.OpenAPIHash)
	}
	if strings.Contains(string(data), "DO_NOT_LEAK") {
		t.Error("cache must never contain secret material")
	}
}
