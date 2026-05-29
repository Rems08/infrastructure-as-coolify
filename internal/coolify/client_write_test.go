package coolify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

type capturedReq struct {
	method string
	path   string
	idemp  string
	body   map[string]any
}

// captureServer records every request and replies with status/respBody for all paths.
func captureServer(t *testing.T, status int, respBody string) (*httptest.Server, *[]capturedReq) {
	t.Helper()
	var got []capturedReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cr := capturedReq{method: r.Method, path: r.URL.Path, idemp: r.Header.Get("Idempotency-Key")}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			_ = json.Unmarshal(raw, &cr.body)
		}
		got = append(got, cr)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func TestCreateApplicationDockerImage(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"app-new-uuid"}`)
	uuid, err := newTestClient(t, srv.URL).CreateApplication(context.Background(), coolify.CreateApplicationRequest{
		BuildPack:               "dockerimage",
		ProjectUUID:             "proj-uuid",
		ServerUUID:              "srv-uuid",
		EnvironmentName:         "staging",
		Name:                    "api",
		Domains:                 "https://api.example.com",
		PortsExposes:            "8000",
		DockerRegistryImageName: "registry/app",
		DockerRegistryImageTag:  "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "app-new-uuid" {
		t.Errorf("uuid = %q, want app-new-uuid", uuid)
	}
	if len(*got) != 1 {
		t.Fatalf("want 1 request, got %d", len(*got))
	}
	req := (*got)[0]
	if req.method != http.MethodPost || req.path != "/api/v1/applications/dockerimage" {
		t.Errorf("got %s %s, want POST /api/v1/applications/dockerimage", req.method, req.path)
	}
	if req.idemp == "" {
		t.Error("create must carry an Idempotency-Key")
	}
	if req.body["docker_registry_image_name"] != "registry/app" || req.body["ports_exposes"] != "8000" {
		t.Errorf("body missing image/port fields: %+v", req.body)
	}
	// The dockerimage endpoint implies the build pack: no build_pack key in the body.
	if _, ok := req.body["build_pack"]; ok {
		t.Errorf("dockerimage body must not carry build_pack: %+v", req.body)
	}
}

func TestCreateApplicationPublicCarriesBuildPack(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"u"}`)
	_, err := newTestClient(t, srv.URL).CreateApplication(context.Background(), coolify.CreateApplicationRequest{
		BuildPack:     "nixpacks",
		ProjectUUID:   "proj-uuid",
		ServerUUID:    "srv-uuid",
		Name:          "web",
		PortsExposes:  "3000",
		GitRepository: "https://github.com/acme/web",
		GitBranch:     "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/applications/public" {
		t.Errorf("path = %q, want /api/v1/applications/public", req.path)
	}
	if req.body["build_pack"] != "nixpacks" {
		t.Errorf("public body build_pack = %v, want nixpacks", req.body["build_pack"])
	}
}

func TestCreateApplicationDockerfileDirect(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"app-df-uuid"}`)
	uuid, err := newTestClient(t, srv.URL).CreateApplication(context.Background(), coolify.CreateApplicationRequest{
		BuildPack:       "dockerfile",
		ProjectUUID:     "proj-uuid",
		ServerUUID:      "srv-uuid",
		EnvironmentName: "staging",
		Name:            "tool",
		PortsExposes:    "8080",
		Dockerfile:      "FROM busybox\nCMD [\"true\"]",
	})
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "app-df-uuid" {
		t.Errorf("uuid = %q, want app-df-uuid", uuid)
	}
	req := (*got)[0]
	if req.method != http.MethodPost || req.path != "/api/v1/applications/dockerfile" {
		t.Errorf("got %s %s, want POST /api/v1/applications/dockerfile", req.method, req.path)
	}
	if req.idemp == "" {
		t.Error("create must carry an Idempotency-Key")
	}
	if req.body["dockerfile"] != "FROM busybox\nCMD [\"true\"]" {
		t.Errorf("dockerfile body not sent verbatim: %+v", req.body)
	}
	if req.body["build_pack"] != "dockerfile" {
		t.Errorf("dockerfile endpoint body build_pack = %v, want dockerfile", req.body["build_pack"])
	}
	// An inline Dockerfile is not base64-encoded (unlike a service compose).
	if _, ok := req.body["git_repository"]; ok {
		t.Errorf("inline dockerfile must not carry git fields: %+v", req.body)
	}
}

func TestCreateApplicationPublicDirect(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"app-pub-uuid"}`)
	uuid, err := newTestClient(t, srv.URL).CreateApplication(context.Background(), coolify.CreateApplicationRequest{
		BuildPack:       "static",
		ProjectUUID:     "proj-uuid",
		ServerUUID:      "srv-uuid",
		EnvironmentName: "production",
		Name:            "site",
		PortsExposes:    "80",
		GitRepository:   "https://github.com/acme/site",
		GitBranch:       "release",
	})
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "app-pub-uuid" {
		t.Errorf("uuid = %q, want app-pub-uuid", uuid)
	}
	req := (*got)[0]
	if req.method != http.MethodPost || req.path != "/api/v1/applications/public" {
		t.Errorf("got %s %s, want POST /api/v1/applications/public", req.method, req.path)
	}
	if req.body["build_pack"] != "static" {
		t.Errorf("public body build_pack = %v, want static", req.body["build_pack"])
	}
	if req.body["git_repository"] != "https://github.com/acme/site" || req.body["git_branch"] != "release" {
		t.Errorf("public body missing git fields: %+v", req.body)
	}
	// environment_name only: v4 environments have no UUID to send.
	if req.body["environment_name"] != "production" {
		t.Errorf("environment_name not sent: %+v", req.body)
	}
	if _, ok := req.body["environment_uuid"]; ok {
		t.Errorf("environment_uuid must be omitted (none resolvable): %+v", req.body)
	}
}

func TestCreateApplicationNotCreatableMakesNoCall(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{}`)
	_, err := newTestClient(t, srv.URL).CreateApplication(context.Background(), coolify.CreateApplicationRequest{
		BuildPack:   "nixpacks", // no git fields -> not creatable from schema
		ProjectUUID: "p",
		ServerUUID:  "s",
		Name:        "web",
	})
	if err == nil {
		t.Fatal("want error for a build_pack the schema cannot create")
	}
	if len(*got) != 0 {
		t.Errorf("no HTTP request should be made when the request is invalid, got %d", len(*got))
	}
}

func TestUpdateApplication(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `{"message":"updated"}`)
	err := newTestClient(t, srv.URL).UpdateApplication(context.Background(), "app-uuid", coolify.UpdateApplicationRequest{
		Domains:      "https://new.example.com",
		PortsExposes: "9000",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.method != http.MethodPatch || req.path != "/api/v1/applications/app-uuid" {
		t.Errorf("got %s %s, want PATCH /api/v1/applications/app-uuid", req.method, req.path)
	}
	if req.idemp == "" {
		t.Error("update must carry an Idempotency-Key")
	}
	if req.body["domains"] != "https://new.example.com" {
		t.Errorf("patch body = %+v", req.body)
	}
	// Empty fields are omitted from the partial patch.
	if _, ok := req.body["docker_registry_image_name"]; ok {
		t.Errorf("empty fields must be omitted from the patch body: %+v", req.body)
	}
}

func TestDeleteApplicationNotFoundIsNoop(t *testing.T) {
	srv, _ := captureServer(t, http.StatusNotFound, `{"message":"not found"}`)
	if err := newTestClient(t, srv.URL).DeleteApplication(context.Background(), "gone"); err != nil {
		t.Errorf("deleting an absent application must be a no-op, got: %v", err)
	}
}

func TestDeleteApplicationServerErrorPropagates(t *testing.T) {
	srv, _ := captureServer(t, http.StatusInternalServerError, `{"message":"boom"}`)
	if err := newTestClient(t, srv.URL).DeleteApplication(context.Background(), "x"); err == nil {
		t.Error("a 500 on delete must propagate")
	}
}

func TestCreateProjectAndEnvironment(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"new-uuid"}`)
	c := newTestClient(t, srv.URL)

	puid, err := c.CreateProject(context.Background(), coolify.CreateProjectRequest{Name: "beenaire", Description: "platform"})
	if err != nil || puid != "new-uuid" {
		t.Fatalf("CreateProject = %q, %v", puid, err)
	}
	euid, err := c.CreateEnvironment(context.Background(), "proj-uuid", coolify.CreateEnvironmentRequest{Name: "staging"})
	if err != nil || euid != "new-uuid" {
		t.Fatalf("CreateEnvironment = %q, %v", euid, err)
	}

	if (*got)[0].path != "/api/v1/projects" || (*got)[0].body["name"] != "beenaire" {
		t.Errorf("project request = %+v", (*got)[0])
	}
	if (*got)[1].path != "/api/v1/projects/proj-uuid/environments" || (*got)[1].body["name"] != "staging" {
		t.Errorf("environment request = %+v", (*got)[1])
	}
	for _, r := range *got {
		if r.idemp == "" {
			t.Errorf("%s %s missing Idempotency-Key", r.method, r.path)
		}
	}
}

func TestDeleteProjectAndEnvironment(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `{"message":"deleted"}`)
	c := newTestClient(t, srv.URL)
	if err := c.DeleteEnvironment(context.Background(), "proj-uuid", "staging"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteProject(context.Background(), "proj-uuid"); err != nil {
		t.Fatal(err)
	}
	if (*got)[0].path != "/api/v1/projects/proj-uuid/environments/staging" || (*got)[0].method != http.MethodDelete {
		t.Errorf("delete env request = %+v", (*got)[0])
	}
	if (*got)[1].path != "/api/v1/projects/proj-uuid" || (*got)[1].method != http.MethodDelete {
		t.Errorf("delete project request = %+v", (*got)[1])
	}
}

// TestIdempotencyKeyStableForIdenticalRequest asserts the key is deterministic, so a CI
// retry of the same create reuses it.
func TestIdempotencyKeyStableForIdenticalRequest(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"u"}`)
	c := newTestClient(t, srv.URL)
	req := coolify.CreateProjectRequest{Name: "beenaire"}
	for i := 0; i < 2; i++ {
		if _, err := c.CreateProject(context.Background(), req); err != nil {
			t.Fatal(err)
		}
	}
	if (*got)[0].idemp != (*got)[1].idemp {
		t.Errorf("identical request produced different Idempotency-Keys: %q vs %q", (*got)[0].idemp, (*got)[1].idemp)
	}
}
