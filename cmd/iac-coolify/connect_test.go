package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/tui"
)

// blankCoolifyEnv sets all four credential vars to empty so the connector's os.Setenv writes are
// restored after the test (t.Setenv records the originals) and no developer shell leaks in.
func blankCoolifyEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"COOLIFY_API_URL", "COOLIFY_API_TOKEN", "CF_ACCESS_CLIENT_ID", "CF_ACCESS_CLIENT_SECRET"} {
		t.Setenv(k, "")
	}
}

func TestConnectFromWizard_ConnectsAndSetsEnv(t *testing.T) {
	blankCoolifyEnv(t)
	srv := importMux(t)

	connect := connectFromWizard(context.Background())
	client, err := connect(tui.ConnectInput{URL: srv.URL, Token: "tok_secret_DO_NOT_LEAK"})
	if err != nil {
		t.Fatalf("connect with valid credentials: %v", err)
	}
	if client == nil {
		t.Fatal("a successful connection must return a client")
	}
	// The token must reach the environment so buildClient/NewFromEnv can source it as a Secret.
	if got := os.Getenv("COOLIFY_API_TOKEN"); got != "tok_secret_DO_NOT_LEAK" {
		t.Errorf("COOLIFY_API_TOKEN = %q, want the entered token", got)
	}
	if got := os.Getenv("COOLIFY_API_URL"); got != srv.URL {
		t.Errorf("COOLIFY_API_URL = %q, want %q", got, srv.URL)
	}
}

func TestConnectFromWizard_FailureDoesNotLeakToken(t *testing.T) {
	blankCoolifyEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	connect := connectFromWizard(context.Background())
	_, err := connect(tui.ConnectInput{URL: srv.URL, Token: "tok_secret_DO_NOT_LEAK"})
	if err == nil {
		t.Fatal("a rejected connection must return an error")
	}
	if strings.Contains(err.Error(), "tok_secret_DO_NOT_LEAK") {
		t.Errorf("connection error leaked the token: %v", err)
	}
}

func TestConnectFromWizard_PopulatesCFAccessEnv(t *testing.T) {
	blankCoolifyEnv(t)
	srv := importMux(t)

	connect := connectFromWizard(context.Background())
	_, err := connect(tui.ConnectInput{
		URL: srv.URL, Token: "tok",
		CFAccessID: "cf-id", CFAccessSecret: "cf-secret-DO_NOT_LEAK",
	})
	if err != nil {
		t.Fatalf("connect with CF-Access: %v", err)
	}
	if got := os.Getenv("CF_ACCESS_CLIENT_ID"); got != "cf-id" {
		t.Errorf("CF_ACCESS_CLIENT_ID = %q, want cf-id", got)
	}
	if got := os.Getenv("CF_ACCESS_CLIENT_SECRET"); got != "cf-secret-DO_NOT_LEAK" {
		t.Errorf("CF_ACCESS_CLIENT_SECRET not set for buildClient to source")
	}
}

// The wizard launches exactly when buildClient reports offline: credentials in the environment
// skip it (browse directly), their absence triggers it.
func TestExplore_WizardGatedByOnlineState(t *testing.T) {
	t.Run("online skips the wizard", func(t *testing.T) {
		blankCoolifyEnv(t)
		srv := importMux(t)
		t.Setenv("COOLIFY_API_TOKEN", "tok")
		t.Setenv("COOLIFY_API_URL", srv.URL)
		_, online, err := buildClient("")
		if err != nil || !online {
			t.Fatalf("credentials in env must be online (online=%v err=%v) so explore skips the wizard", online, err)
		}
	})
	t.Run("offline triggers the wizard", func(t *testing.T) {
		blankCoolifyEnv(t)
		os.Unsetenv("COOLIFY_API_URL")
		os.Unsetenv("COOLIFY_API_TOKEN")
		_, online, err := buildClient("")
		if err != nil {
			t.Fatalf("offline buildClient errored: %v", err)
		}
		if online {
			t.Fatal("no credentials must report offline so explore launches the wizard")
		}
	})
}
