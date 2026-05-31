package coolify_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

func TestListApplicationEnvs(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `[{"uuid":"e1","key":"NODE_ENV","value":"prod"}]`)
	envs, err := newTestClient(t, srv.URL).ListApplicationEnvs(context.Background(), "app-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Key != "NODE_ENV" || envs[0].Value != "prod" {
		t.Errorf("list = %+v, want one NODE_ENV=prod", envs)
	}
	if (*got)[0].method != http.MethodGet || (*got)[0].path != "/api/v1/applications/app-1/envs" {
		t.Errorf("got %s %s, want GET .../app-1/envs", (*got)[0].method, (*got)[0].path)
	}
}

func TestListApplicationEnvsDecodesScope(t *testing.T) {
	// The live response carries is_buildtime/is_runtime/is_preview per entry, and returns the
	// same key once per scope. The fixture mirrors that shape (not the pinned spec, which omits
	// the scope flags).
	body := `[
		{"uuid":"e1","key":"NODE_ENV","value":"prod","is_buildtime":true,"is_runtime":true,"is_preview":false},
		{"uuid":"e2","key":"GIT_SHA","value":"abc","is_buildtime":true,"is_runtime":false},
		{"uuid":"e3","key":"GIT_SHA","value":"abc","is_buildtime":false,"is_runtime":true}
	]`
	srv, _ := captureServer(t, http.StatusOK, body)
	envs, err := newTestClient(t, srv.URL).ListApplicationEnvs(context.Background(), "app-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 3 {
		t.Fatalf("list = %d entries, want 3 (same-key scope variants both kept)", len(envs))
	}
	if !envs[0].IsBuildtime || !envs[0].IsRuntime {
		t.Errorf("NODE_ENV scope = build:%v runtime:%v, want both true", envs[0].IsBuildtime, envs[0].IsRuntime)
	}
	if !envs[1].IsBuildtime || envs[1].IsRuntime {
		t.Errorf("GIT_SHA build variant = build:%v runtime:%v, want build-only", envs[1].IsBuildtime, envs[1].IsRuntime)
	}
	if envs[2].IsBuildtime || !envs[2].IsRuntime {
		t.Errorf("GIT_SHA runtime variant = build:%v runtime:%v, want runtime-only", envs[2].IsBuildtime, envs[2].IsRuntime)
	}
	// The value stays a plain maskable string on read; Secret is never populated from a response.
	if envs[0].Value != "prod" || !envs[0].Secret.IsZero() {
		t.Errorf("value/secret = %q/%v, want plain prod and zero secret", envs[0].Value, envs[0].Secret)
	}
}

func TestBulkUpdateApplicationEnvsRevealsSecretAtBoundary(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `[]`)
	t.Setenv("DB_PASS", "s3cr3t-value")
	sec, err := secrets.NewFromEnv("DB_PASS")
	if err != nil {
		t.Fatal(err)
	}
	err = newTestClient(t, srv.URL).BulkUpdateApplicationEnvs(context.Background(), "app-1", []coolify.ServiceEnvVar{
		{Key: "NODE_ENV", Value: "prod"},
		{Key: "DATABASE_URL", Secret: sec},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.method != http.MethodPatch || req.path != "/api/v1/applications/app-1/envs/bulk" {
		t.Fatalf("got %s %s, want PATCH .../app-1/envs/bulk", req.method, req.path)
	}
	if req.idemp == "" {
		t.Error("bulk update must carry an Idempotency-Key")
	}
	data, ok := req.body["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("bulk body data = %+v, want 2 entries", req.body["data"])
	}
	// The secret is revealed only at the HTTP boundary — it is what gets deployed.
	second, _ := data[1].(map[string]any)
	if second["key"] != "DATABASE_URL" || second["value"] != "s3cr3t-value" {
		t.Errorf("bulk secret env not revealed at boundary: %+v", second)
	}
}

func TestDeleteApplicationEnvIgnores404(t *testing.T) {
	srv, got := captureServer(t, http.StatusNotFound, `{"message":"not found"}`)
	if err := newTestClient(t, srv.URL).DeleteApplicationEnv(context.Background(), "app-1", "gone"); err != nil {
		t.Errorf("DeleteApplicationEnv on a 404 must be a no-op, got %v", err)
	}
	if (*got)[0].method != http.MethodDelete || (*got)[0].path != "/api/v1/applications/app-1/envs/gone" {
		t.Errorf("got %s %s, want DELETE .../app-1/envs/gone", (*got)[0].method, (*got)[0].path)
	}
}

func TestApplicationEnvsRejectEmptyUUID(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `[]`)
	c := newTestClient(t, srv.URL)
	if _, err := c.ListApplicationEnvs(context.Background(), ""); err == nil {
		t.Error("ListApplicationEnvs must reject an empty uuid")
	}
	if err := c.BulkUpdateApplicationEnvs(context.Background(), "", nil); err == nil {
		t.Error("BulkUpdateApplicationEnvs must reject an empty uuid")
	}
	if err := c.DeleteApplicationEnv(context.Background(), "", "e1"); err == nil {
		t.Error("DeleteApplicationEnv must reject an empty uuid")
	}
	if len(*got) != 0 {
		t.Errorf("an empty uuid must not reach the API, got %d calls", len(*got))
	}
}
