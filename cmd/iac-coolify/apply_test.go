package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func fullProjectDir() string { return filepath.Join("..", "..", "examples", "full-project") }
func openapiDir() string     { return filepath.Join("..", "..", "testdata", "openapi") }

func TestApplyCommand_DryRunOffline(t *testing.T) {
	clearCoolifyEnv(t)
	out, err := runCmd(t, "apply", fullProjectDir(), "--dry-run", "--output=json")
	if err != nil {
		t.Fatalf("dry-run apply: %v (out: %s)", err, out)
	}
	var got struct {
		DryRun  bool `json:"dry_run"`
		ToAdd   int  `json:"to_add"`
		Applied int  `json:"applied"`
	}
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse json: %v\n%s", jErr, out)
	}
	if !got.DryRun || got.ToAdd != 3 || got.Applied != 0 {
		t.Errorf("dry-run output = %+v, want dryRun true, toAdd 3, applied 0", got)
	}
}

func TestApplyCommand_DryRunBuildsServiceOp(t *testing.T) {
	clearCoolifyEnv(t)
	t.Setenv("GRAFANA_ADMIN_PASSWORD", "from-env")
	fullStack := filepath.Join("..", "..", "examples", "full-stack")
	out, err := runCmd(t, "apply", fullStack, "--dry-run", "--output=json")
	if err != nil {
		t.Fatalf("dry-run apply: %v (out: %s)", err, out)
	}
	var got struct {
		ToAdd      int      `json:"to_add"`
		Operations []string `json:"operations"`
	}
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse json: %v\n%s", jErr, out)
	}
	// project + environment + service + 3 applications, with the service and applications
	// ordered after the project and environment they depend on.
	if got.ToAdd != 6 {
		t.Errorf("to_add = %d, want 6 (project + environment + service + 3 applications)", got.ToAdd)
	}
	svc := indexOfContaining(got.Operations, "Service/beenaire/production/observability-stack")
	proj := indexOfContaining(got.Operations, "Project/beenaire")
	env := indexOfContaining(got.Operations, "Environment/beenaire/production")
	if svc < 0 || proj < 0 || env < 0 {
		t.Fatalf("missing expected ops in %v", got.Operations)
	}
	if svc < proj || svc < env {
		t.Errorf("service must be ordered after its project and environment: %v", got.Operations)
	}
}

func indexOfContaining(ops []string, sub string) int {
	for i, op := range ops {
		if strings.Contains(op, sub) {
			return i
		}
	}
	return -1
}

func TestApplyCommand_NonInteractiveRefuses(t *testing.T) {
	clearCoolifyEnv(t)
	_, err := runCmd(t, "apply", fullProjectDir(), "--output=json")
	if err == nil {
		t.Fatal("apply must refuse in a non-interactive session without --auto-approve")
	}
	if !strings.Contains(err.Error(), "auto-approve") {
		t.Errorf("error = %v, want it to mention --auto-approve", err)
	}
}

func TestApplyCommand_ParallelismRejected(t *testing.T) {
	clearCoolifyEnv(t)
	_, err := runCmd(t, "apply", fullProjectDir(), "--parallelism=4", "--auto-approve", "--dry-run")
	if err == nil || !strings.Contains(err.Error(), "parallelism") {
		t.Errorf("want a parallelism error, got %v", err)
	}
}

// applyMux serves an empty live instance plus the write endpoints, recording requests.
func applyMux(t *testing.T, failPath string) (*httptest.Server, *[]string) {
	t.Helper()
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == failPath:
			w.WriteHeader(http.StatusBadRequest) // non-retryable: keeps the test fast
			_, _ = w.Write([]byte(`{"message":"boom"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/servers":
			_, _ = w.Write([]byte(`[{"uuid":"srv-uuid","name":"localhost"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/applications":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/services":
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/resources"):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			_, _ = w.Write([]byte(`{"uuid":"proj-uuid"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/environments"):
			_, _ = w.Write([]byte(`{"uuid":"env-uuid"}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/applications/"):
			_, _ = w.Write([]byte(`{"uuid":"app-uuid"}`))
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestApplyCommand_OnlineCreateFromScratch(t *testing.T) {
	clearCoolifyEnv(t)
	srv, calls := applyMux(t, "")
	t.Setenv("COOLIFY_API_TOKEN", "tok_DO_NOT_LEAK")
	auditLog := filepath.Join(t.TempDir(), "audit.log")

	out, err := runCmd(t, "apply", fullProjectDir(),
		"--coolify-url", srv.URL, "--auto-approve", "--output=json",
		"--openapi-dir", openapiDir(), "--audit-log", auditLog)
	if err != nil {
		t.Fatalf("online apply: %v\n%s", err, out)
	}
	var got struct {
		Applied   int `json:"applied"`
		ToAdd     int `json:"to_add"`
		ToDestroy int `json:"to_destroy"`
	}
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse json: %v\n%s", jErr, out)
	}
	if got.Applied != 3 || got.ToAdd != 3 {
		t.Errorf("summary = %+v, want 3 applied / 3 to add", got)
	}
	// Writes happen project → environment → application.
	var writes []string
	for _, c := range *calls {
		if strings.HasPrefix(c, "POST") {
			writes = append(writes, c)
		}
	}
	want := []string{"POST /api/v1/projects", "POST /api/v1/projects/proj-uuid/environments", "POST /api/v1/applications/dockerimage"}
	if strings.Join(writes, ",") != strings.Join(want, ",") {
		t.Errorf("write order = %v, want %v", writes, want)
	}
	if strings.Contains(out, "tok_DO_NOT_LEAK") {
		t.Error("apply output leaks the token")
	}
}

func TestApplyCommand_PartialFailureExitsTwo(t *testing.T) {
	clearCoolifyEnv(t)
	// Environment create fails after the project create succeeded → partial → exit 2.
	srv, _ := applyMux(t, "/api/v1/projects/proj-uuid/environments")
	t.Setenv("COOLIFY_API_TOKEN", "tok")

	_, err := runCmd(t, "apply", fullProjectDir(),
		"--coolify-url", srv.URL, "--auto-approve", "--output=json",
		"--openapi-dir", openapiDir(), "--audit-log", filepath.Join(t.TempDir(), "audit.log"))
	if err == nil {
		t.Fatal("partial apply must surface an error")
	}
	var ec interface{ ExitCode() int }
	if !asExit(err, &ec) || ec.ExitCode() != 2 {
		t.Fatalf("want exit code 2 (partial), got %v", err)
	}
}
