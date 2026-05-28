package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearCoolifyEnv removes ambient Coolify/CF credentials so tests are deterministic
// regardless of the developer's shell.
func clearCoolifyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"COOLIFY_API_TOKEN", "COOLIFY_API_URL", "CF_ACCESS_CLIENT_ID", "CF_ACCESS_CLIENT_SECRET", "CI"} {
		if v, ok := os.LookupEnv(k); ok {
			orig := v
			_ = os.Unsetenv(k)
			t.Cleanup(func() { _ = os.Setenv(k, orig) })
		}
	}
}

func minimalDir() string { return filepath.Join("..", "..", "examples", "minimal") }

func TestPlanCommand_OfflineCreatesAll(t *testing.T) {
	clearCoolifyEnv(t)
	out, err := runCmd(t, "plan", minimalDir(), "--output=json")
	if err != nil {
		t.Fatalf("offline plan: %v (out: %s)", err, out)
	}
	var got struct {
		Summary struct{ Add, Change, Destroy int } `json:"summary"`
		Changes []map[string]any                   `json:"changes"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse json: %v\n%s", err, out)
	}
	if got.Summary.Add != 1 || got.Summary.Change != 0 {
		t.Errorf("summary = %+v, want add1 change0", got.Summary)
	}
	if len(got.Changes) == 0 {
		t.Error("offline plan must report changes")
	}
}

func TestPlanCommand_HalfConfiguredFails(t *testing.T) {
	clearCoolifyEnv(t)
	t.Setenv("COOLIFY_API_TOKEN", "tok") // URL missing → misconfiguration
	if _, err := runCmd(t, "plan", minimalDir(), "--output=json"); err == nil {
		t.Fatal("want error when only the token is set")
	}
}

// coolifyMux serves the documented endpoints the plan path uses. fqdn drives noop vs drift.
func coolifyMux(t *testing.T, fqdn string) *httptest.Server {
	t.Helper()
	app := `{"uuid":"u-web","name":"web","environment_id":10,` +
		`"fqdn":"` + fqdn + `","ports_exposes":"3000",` +
		`"docker_registry_image_name":"registry.example.com/demo/web","docker_registry_image_tag":"v1-0-0"}`
	bodies := map[string]string{
		"/api/v1/projects":                 `[{"id":1,"uuid":"p1","name":"demo"}]`,
		"/api/v1/projects/p1/environments": `[{"id":10,"name":"staging","project_id":1}]`,
		"/api/v1/servers":                  `[{"uuid":"srv-1","name":"localhost"}]`,
		"/api/v1/applications":             `[` + app + `]`,
		"/api/v1/applications/u-web":       app,
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

func TestPlanCommand_OnlineNoop(t *testing.T) {
	clearCoolifyEnv(t)
	srv := coolifyMux(t, "https://web.example.com") // matches examples/minimal
	t.Setenv("COOLIFY_API_TOKEN", "tok_DO_NOT_LEAK")
	out, err := runCmd(t, "plan", minimalDir(),
		"--coolify-url", srv.URL, "--output=text", "--detailed-exitcode",
		"--openapi-dir", filepath.Join("..", "..", "testdata", "openapi"))
	if err != nil {
		t.Fatalf("online noop plan should exit 0, got err %v\n%s", err, out)
	}
	if !strings.Contains(out, "no changes") {
		t.Errorf("expected no-changes output, got:\n%s", out)
	}
	if strings.Contains(out, "tok_DO_NOT_LEAK") {
		t.Error("plan output leaks the token")
	}
}

func TestPlanCommand_OnlineUpdateExitsTwo(t *testing.T) {
	clearCoolifyEnv(t)
	srv := coolifyMux(t, "https://stale.example.com") // drifted fqdn
	t.Setenv("COOLIFY_API_TOKEN", "tok")
	out, err := runCmd(t, "plan", minimalDir(), "--coolify-url", srv.URL, "--output=text", "--detailed-exitcode")
	if err == nil {
		t.Fatalf("drifted plan with --detailed-exitcode must error (exit 2); out:\n%s", out)
	}
	var ec interface{ ExitCode() int }
	if !asExit(err, &ec) || ec.ExitCode() != 2 {
		t.Fatalf("want exit code 2, got err %v", err)
	}
	if !strings.Contains(out, "will be updated") {
		t.Errorf("expected update output, got:\n%s", out)
	}
}

// asExit reports whether err carries an ExitCode (mirrors errors.As without importing it
// just for the test signature).
func asExit(err error, target *interface{ ExitCode() int }) bool {
	if e, ok := err.(interface{ ExitCode() int }); ok {
		*target = e
		return true
	}
	return false
}
