package coolify_test

import (
	"context"
	"net/http"
	"testing"
)

func TestApplicationLifecycle(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `{}`)
	c := newTestClient(t, srv.URL)
	ctx := context.Background()

	for _, step := range []func(context.Context, string) error{
		c.StartApplication, c.StopApplication, c.RestartApplication,
	} {
		if err := step(ctx, "app-1"); err != nil {
			t.Fatal(err)
		}
	}

	want := []string{
		"GET /api/v1/applications/app-1/start",
		"GET /api/v1/applications/app-1/stop",
		"GET /api/v1/applications/app-1/restart",
	}
	for i, w := range want {
		gotLine := (*got)[i].method + " " + (*got)[i].path
		if gotLine != w {
			t.Errorf("call[%d] = %q, want %q", i, gotLine, w)
		}
	}
}

func TestApplicationLifecyclePropagatesAPIError(t *testing.T) {
	srv, _ := captureServer(t, http.StatusInternalServerError, `{"message":"boom"}`)
	if err := newTestClient(t, srv.URL).StartApplication(context.Background(), "app-1"); err == nil {
		t.Error("StartApplication must propagate a non-2xx response as an error")
	}
}

func TestApplicationLifecycleRejectsEmptyUUID(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `{}`)
	if err := newTestClient(t, srv.URL).RestartApplication(context.Background(), ""); err == nil {
		t.Error("RestartApplication must reject an empty uuid")
	}
	if len(*got) != 0 {
		t.Errorf("an empty uuid must not reach the API, got %d calls", len(*got))
	}
}
