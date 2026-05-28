package state_test

import (
	"context"
	"encoding/json"
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
			{"id":10,"name":"staging","project_id":1},
			{"id":11,"name":"production","project_id":1}
		]`,
		"/api/v1/projects/proj-lab/environments": `[
			{"id":20,"name":"staging","project_id":2}
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

func TestResolveEmpty(t *testing.T) {
	bodies := map[string]string{
		"/api/v1/projects":     `[]`,
		"/api/v1/servers":      `[]`,
		"/api/v1/applications": `[]`,
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
