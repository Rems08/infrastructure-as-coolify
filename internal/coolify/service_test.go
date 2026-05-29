package coolify_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

func TestCreateServiceBase64EncodesCompose(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"svc-new","domains":["https://x"]}`)
	const compose = "services:\n  app:\n    image: nginx:1.27\n"
	uuid, err := newTestClient(t, srv.URL).CreateService(context.Background(), coolify.CreateServiceRequest{
		Name:             "obs",
		ProjectUUID:      "proj-uuid",
		EnvironmentName:  "production",
		EnvironmentUUID:  "env-uuid",
		ServerUUID:       "srv-uuid",
		DockerComposeRaw: compose,
	})
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "svc-new" {
		t.Errorf("uuid = %q, want svc-new", uuid)
	}
	body := (*got)[0].body
	encoded, ok := body["docker_compose_raw"].(string)
	if !ok {
		t.Fatalf("docker_compose_raw missing or not a string: %+v", body)
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("docker_compose_raw is not valid base64: %v", err)
	}
	if string(decoded) != compose {
		t.Errorf("base64 round-trip = %q, want %q", decoded, compose)
	}
	if (*got)[0].path != "/api/v1/services" || (*got)[0].idemp == "" {
		t.Errorf("want POST /services with Idempotency-Key, got %+v", (*got)[0])
	}
}

func TestCreateServiceSendsBothEnvNameAndUUID(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"svc-new"}`)
	_, err := newTestClient(t, srv.URL).CreateService(context.Background(), coolify.CreateServiceRequest{
		Name:            "gitea",
		Type:            "gitea-with-mysql",
		ProjectUUID:     "proj-uuid",
		EnvironmentName: "staging",
		EnvironmentUUID: "env-uuid",
		ServerUUID:      "srv-uuid",
	})
	if err != nil {
		t.Fatal(err)
	}
	body := (*got)[0].body
	if body["environment_name"] != "staging" || body["environment_uuid"] != "env-uuid" {
		t.Errorf("create must send both environment_name and environment_uuid: %+v", body)
	}
	if body["type"] != "gitea-with-mysql" {
		t.Errorf("type passthrough missing: %+v", body)
	}
	if _, present := body["docker_compose_raw"]; present {
		t.Errorf("type mode must not send docker_compose_raw: %+v", body)
	}
}

func TestServiceClientHeadersIncludeCFAccess(t *testing.T) {
	var hdr http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"uuid":"svc-new"}`)
	}))
	t.Cleanup(srv.Close)

	t.Setenv("COOLIFY_API_TOKEN", "tok_svc")
	t.Setenv("CF_SECRET", "cf-secret-value")
	tok, err := secrets.NewFromEnv("COOLIFY_API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	cfSec, err := secrets.NewFromEnv("CF_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	c, err := coolify.NewClient(coolify.Options{
		BaseURL: srv.URL, Token: tok,
		CFAccessClientID: "cf-id", CFAccessClientSecret: cfSec,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateService(context.Background(), coolify.CreateServiceRequest{
		Type: "gitea-with-mysql", ProjectUUID: "p", ServerUUID: "s",
	}); err != nil {
		t.Fatal(err)
	}
	if hdr.Get("Authorization") != "Bearer tok_svc" {
		t.Errorf("Authorization = %q", hdr.Get("Authorization"))
	}
	if hdr.Get("CF-Access-Client-Id") != "cf-id" || hdr.Get("CF-Access-Client-Secret") != "cf-secret-value" {
		t.Errorf("CF Access headers missing: id=%q secret-set=%v", hdr.Get("CF-Access-Client-Id"), hdr.Get("CF-Access-Client-Secret") != "")
	}
}

func TestServiceEnvsCRUD(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `[{"uuid":"e1","key":"K","value":"V"}]`)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	envs, err := c.ListServiceEnvs(ctx, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Key != "K" || envs[0].Value != "V" {
		t.Errorf("list = %+v, want one K=V", envs)
	}
	if err := c.CreateServiceEnv(ctx, "svc", coolify.ServiceEnvVar{Key: "NODE_ENV", Value: "prod"}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateServiceEnv(ctx, "svc", coolify.ServiceEnvVar{Key: "NODE_ENV", Value: "staging"}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteServiceEnv(ctx, "svc", "e1"); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"GET /api/v1/services/svc/envs",
		"POST /api/v1/services/svc/envs",
		"PATCH /api/v1/services/svc/envs",
		"DELETE /api/v1/services/svc/envs/e1",
	}
	for i, w := range want {
		gotLine := (*got)[i].method + " " + (*got)[i].path
		if gotLine != w {
			t.Errorf("call[%d] = %q, want %q", i, gotLine, w)
		}
	}
	if (*got)[1].body["key"] != "NODE_ENV" || (*got)[1].body["value"] != "prod" {
		t.Errorf("create env body = %+v", (*got)[1].body)
	}
}

func TestListServices(t *testing.T) {
	srv, _ := captureServer(t, http.StatusOK, `[{"uuid":"svc-1","name":"obs","environment_id":11}]`)
	services, err := newTestClient(t, srv.URL).ListServices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].UUID != "svc-1" || services[0].EnvironmentID != 11 {
		t.Errorf("ListServices = %+v, want one svc-1 in env 11", services)
	}
}

func TestServiceUpdateDeleteLifecycle(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `{"uuid":"svc-1"}`)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()
	const compose = "services:\n  app:\n    image: caddy:2\n"
	if err := c.UpdateService(ctx, "svc-1", coolify.UpdateServiceRequest{Name: "obs", DockerComposeRaw: compose}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []func(context.Context, string) error{c.StartService, c.StopService, c.RestartService, c.DeleteService} {
		if err := step(ctx, "svc-1"); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"PATCH /api/v1/services/svc-1",
		"POST /api/v1/services/svc-1/start",
		"POST /api/v1/services/svc-1/stop",
		"POST /api/v1/services/svc-1/restart",
		"DELETE /api/v1/services/svc-1",
	}
	for i, w := range want {
		gotLine := (*got)[i].method + " " + (*got)[i].path
		if gotLine != w {
			t.Errorf("call[%d] = %q, want %q", i, gotLine, w)
		}
		if (*got)[i].idemp == "" {
			t.Errorf("%s missing Idempotency-Key", w)
		}
	}
	// The update base64-encodes the compose content.
	encoded, _ := (*got)[0].body["docker_compose_raw"].(string)
	if decoded, dErr := base64.StdEncoding.DecodeString(encoded); dErr != nil || string(decoded) != compose {
		t.Errorf("update compose not base64 round-trip: %v", dErr)
	}
}

func TestDeleteServiceIgnores404(t *testing.T) {
	srv, _ := captureServer(t, http.StatusNotFound, `{"message":"not found"}`)
	if err := newTestClient(t, srv.URL).DeleteService(context.Background(), "gone"); err != nil {
		t.Errorf("DeleteService on a 404 must be a no-op, got %v", err)
	}
}

func TestServiceEnvsBulkUpdate(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `[]`)
	t.Setenv("DB_PASS", "s3cr3t-value")
	sec, err := secrets.NewFromEnv("DB_PASS")
	if err != nil {
		t.Fatal(err)
	}
	err = newTestClient(t, srv.URL).BulkUpdateServiceEnvs(context.Background(), "svc", []coolify.ServiceEnvVar{
		{Key: "NODE_ENV", Value: "prod"},
		{Key: "DATABASE_URL", Secret: sec},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.method != http.MethodPatch || req.path != "/api/v1/services/svc/envs/bulk" {
		t.Fatalf("got %s %s, want PATCH .../envs/bulk", req.method, req.path)
	}
	data, ok := req.body["data"].([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("bulk body data = %+v, want 2 entries", req.body["data"])
	}
	// The secret value is revealed at the HTTP boundary (it is what gets deployed).
	second, _ := data[1].(map[string]any)
	if second["key"] != "DATABASE_URL" || second["value"] != "s3cr3t-value" {
		t.Errorf("bulk secret env not revealed at boundary: %+v", second)
	}
}
