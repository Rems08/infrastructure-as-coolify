package apply_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

type wroteReq struct {
	method string
	path   string
	idemp  string
	auth   string
	body   map[string]any
}

// e2eServer records every request and replies to the write endpoints. failPath, when set,
// makes that exact path return 500 (to exercise mid-apply failure). Create endpoints
// return a uuid derived from the path so threaded parent UUIDs are observable.
func e2eServer(t *testing.T, failPath string) (*httptest.Server, *[]wroteReq) {
	t.Helper()
	var reqs []wroteReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wr := wroteReq{method: r.Method, path: r.URL.Path, idemp: r.Header.Get("Idempotency-Key"), auth: r.Header.Get("Authorization")}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &wr.body)
		}
		reqs = append(reqs, wr)

		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == failPath {
			// 400 is a non-retryable client error, so the test exercises the engine's
			// stop-on-failure path without waiting on the HTTP retry backoff.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/projects":
			_, _ = w.Write([]byte(`{"uuid":"proj-uuid"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/environments"):
			_, _ = w.Write([]byte(`{"uuid":"env-uuid"}`))
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"uuid":"app-uuid"}`))
		default: // PATCH / DELETE
			_, _ = w.Write([]byte(`{"message":"ok"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

func e2eClient(t *testing.T, baseURL string) *coolify.Client {
	t.Helper()
	t.Setenv("COOLIFY_API_TOKEN", "tok_e2e_DO_NOT_LEAK")
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

func fromScratchOps(t *testing.T) []apply.Operation {
	t.Helper()
	ops, err := apply.OrderApply([]apply.Operation{
		apply.CreateProjectOp(resource.Project{Metadata: resource.ProjectMeta{Name: "beenaire"}}),
		apply.CreateEnvironmentOp(resource.Environment{Metadata: resource.EnvironmentMeta{Name: "staging", Project: "beenaire"}}),
		apply.ApplicationOp(apply.OpCreate, resource.Application{
			Metadata: resource.ApplicationMeta{Name: "api", Project: "beenaire", Environment: "staging"},
			Spec: resource.ApplicationSpec{
				BuildPack:   "dockerimage",
				Image:       &resource.ImageSpec{Name: "registry/api", Tag: "v1"},
				Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
				FQDN:        "https://api.example.com",
				Port:        8000,
			},
		}, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	return ops
}

func serverOnly() state.Map {
	return state.Map{state.ResourceKey{Kind: state.KindServer, Name: "localhost"}: "srv-uuid"}
}

func paths(reqs []wroteReq) []string {
	out := make([]string, len(reqs))
	for i, r := range reqs {
		out[i] = r.method + " " + r.path
	}
	return out
}

func TestApplyFromScratchHttpTest(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	eng := apply.NewEngine(e2eClient(t, srv.URL), serverOnly(), apply.NewAuditor(auditPath))

	sum, err := eng.Apply(context.Background(), fromScratchOps(t))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if sum.Applied != 3 {
		t.Fatalf("applied = %d, want 3", sum.Applied)
	}

	want := []string{
		"POST /api/v1/projects",
		"POST /api/v1/projects/proj-uuid/environments", // threaded project UUID from the create response
		"POST /api/v1/applications/dockerimage",
	}
	got := paths(*reqs)
	if len(got) != len(want) {
		t.Fatalf("requests = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// The application body must carry the resolved server UUID and project UUID.
	appBody := (*reqs)[2].body
	if appBody["server_uuid"] != "srv-uuid" || appBody["project_uuid"] != "proj-uuid" {
		t.Errorf("application body not wired from resolved+threaded state: %+v", appBody)
	}

	// Audit: one line per applied operation, never the token.
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; n != 3 {
		t.Errorf("audit log has %d lines, want 3", n)
	}
	if strings.Contains(string(data), "tok_e2e") {
		t.Error("audit log leaked the token")
	}
}

func TestApplyIdempotencyKeyHeaderPresent(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	eng := apply.NewEngine(e2eClient(t, srv.URL), serverOnly(), nil)
	if _, err := eng.Apply(context.Background(), fromScratchOps(t)); err != nil {
		t.Fatal(err)
	}
	if len(*reqs) == 0 {
		t.Fatal("no requests recorded")
	}
	for _, r := range *reqs {
		if r.idemp == "" {
			t.Errorf("%s %s missing Idempotency-Key", r.method, r.path)
		}
		if r.auth != "Bearer tok_e2e_DO_NOT_LEAK" {
			t.Errorf("%s %s missing/incorrect Authorization header", r.method, r.path)
		}
	}
}

func TestApplyE2EUpdatePartial(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	resolved := serverOnly()
	resolved[state.ResourceKey{Project: "beenaire", Environment: "staging", Kind: resource.KindApplication, Name: "api"}] = "app-uuid-123"
	eng := apply.NewEngine(e2eClient(t, srv.URL), resolved, nil)

	app := resource.Application{Metadata: resource.ApplicationMeta{Name: "api", Project: "beenaire", Environment: "staging"}}
	changes := []plan.Change{{Op: plan.OpUpdate, Path: "Application.api.fqdn", New: "https://new.example.com"}}
	if _, err := eng.Apply(context.Background(), []apply.Operation{apply.ApplicationOp(apply.OpUpdate, app, changes)}); err != nil {
		t.Fatal(err)
	}
	if got := paths(*reqs); len(got) != 1 || got[0] != "PATCH /api/v1/applications/app-uuid-123" {
		t.Fatalf("requests = %v, want one PATCH to the resolved uuid", got)
	}
	if (*reqs)[0].body["domains"] != "https://new.example.com" {
		t.Errorf("patch body = %+v, want domains mapped from the fqdn change", (*reqs)[0].body)
	}
}

func TestApplyE2EDeleteReverseOrder(t *testing.T) {
	srv, reqs := e2eServer(t, "")
	resolved := state.Map{
		state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}:                                             "proj-uuid",
		state.ResourceKey{Project: "beenaire", Kind: resource.KindEnvironment, Name: "staging"}:                     "staging",
		state.ResourceKey{Project: "beenaire", Environment: "staging", Kind: resource.KindApplication, Name: "api"}: "app-uuid",
	}
	ops, err := apply.OrderDelete([]apply.Operation{
		{Op: apply.OpDelete, Kind: resource.KindProject, Name: "beenaire"},
		{Op: apply.OpDelete, Kind: resource.KindEnvironment, Project: "beenaire", Name: "staging"},
		{Op: apply.OpDelete, Kind: resource.KindApplication, Project: "beenaire", Environment: "staging", Name: "api"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := apply.NewEngine(e2eClient(t, srv.URL), resolved, nil).Apply(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DELETE /api/v1/applications/app-uuid",
		"DELETE /api/v1/projects/proj-uuid/environments/staging",
		"DELETE /api/v1/projects/proj-uuid",
	}
	got := paths(*reqs)
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("delete order = %v, want %v", got, want)
		}
	}
}

func TestApplyE2EPartialFailureMidApply(t *testing.T) {
	// The environment create fails; the project create already succeeded.
	srv, reqs := e2eServer(t, "/api/v1/projects/proj-uuid/environments")
	eng := apply.NewEngine(e2eClient(t, srv.URL), serverOnly(), nil)

	sum, err := eng.Apply(context.Background(), fromScratchOps(t))
	if err == nil {
		t.Fatal("want error when an operation fails mid-apply")
	}
	if sum.Applied != 1 || sum.Failed != 1 {
		t.Errorf("summary = %+v, want 1 applied 1 failed (partial)", sum)
	}
	// The application (3rd op) must never be attempted after the failure.
	for _, r := range *reqs {
		if strings.Contains(r.path, "/applications/") {
			t.Errorf("application create attempted after a mid-apply failure: %v", paths(*reqs))
		}
	}
}
