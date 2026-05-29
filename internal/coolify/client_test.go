package coolify_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

const testToken = "tok_live_abc123_DO_NOT_LEAK"

func newTestClient(t *testing.T, baseURL string) *coolify.Client {
	t.Helper()
	t.Setenv("COOLIFY_API_TOKEN", testToken)
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

func TestNewClient_Validation(t *testing.T) {
	t.Setenv("COOLIFY_API_TOKEN", testToken)
	tok, err := secrets.NewFromEnv("COOLIFY_API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		opts    coolify.Options
		wantErr bool
	}{
		{name: "ok", opts: coolify.Options{BaseURL: "https://x", Token: tok}},
		{name: "missing url", opts: coolify.Options{Token: tok}, wantErr: true},
		{name: "missing token", opts: coolify.Options{BaseURL: "https://x"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := coolify.NewClient(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewClient err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestListApplications_OK(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "coolify_list_apps.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/v1/applications" {
			t.Errorf("path = %q, want /api/v1/applications", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(golden)
	}))
	defer srv.Close()

	apps, err := newTestClient(t, srv.URL).ListApplications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 2 {
		t.Fatalf("got %d apps, want 2", len(apps))
	}
	if apps[0].UUID != "hzw53gga4fcgpsl706h5rgmo" || apps[0].Name != "Beenaire Back Office" {
		t.Errorf("unexpected first app: %+v", apps[0])
	}
	if apps[1].PortsExposes != "8000" {
		t.Errorf("apps[1].PortsExposes = %q, want 8000", apps[1].PortsExposes)
	}
	if gotAuth != "Bearer "+testToken {
		t.Errorf("Authorization = %q, want Bearer %s", gotAuth, testToken)
	}
}

func TestListApplications_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).ListApplications(context.Background())
	if err == nil {
		t.Fatal("want error on 401")
	}
	var apiErr *coolify.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want *coolify.APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	// Token must never appear in an error.
	if strings.Contains(err.Error(), testToken) {
		t.Error("error message leaks the token")
	}
}

func TestListApplications_RateLimitedRetries(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "0") // keep the test fast; exercises the retry path
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).ListApplications(context.Background())
	if err == nil {
		t.Fatal("want error on persistent 429")
	}
	var apiErr *coolify.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("want APIError 429, got %v", err)
	}
	// RetryMax=3 → 1 initial + 3 retries = 4 attempts.
	if got := atomic.LoadInt32(&attempts); got != 4 {
		t.Errorf("attempts = %d, want 4 (1 + RetryMax 3)", got)
	}
}

func TestGetServerResources_OK(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "server_resources.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(golden)
	}))
	defer srv.Close()

	res, err := newTestClient(t, srv.URL).GetServerResources(context.Background(), "srv-abc")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/servers/srv-abc/resources" {
		t.Errorf("path = %q, want /api/v1/servers/srv-abc/resources", gotPath)
	}
	if len(res) != 9 {
		t.Fatalf("got %d resources, want 9", len(res))
	}
	// The typed array decodes the discriminating `type` field, not an opaque blob.
	var standalone int
	for _, r := range res {
		if strings.HasPrefix(r.Type, "standalone-") {
			standalone++
		}
	}
	if standalone != 4 {
		t.Errorf("standalone resources = %d, want 4", standalone)
	}
	if res[3].Name != "pg-restaurant-core-api-staging" || res[3].UUID != "t50fefd4yb1salodq9bipiw3" {
		t.Errorf("unexpected resource[3]: %+v", res[3])
	}
}

func TestGetDatabase_decodesObservedShape(t *testing.T) {
	golden, err := os.ReadFile(filepath.Join("testdata", "database-singular.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(golden)
	}))
	defer srv.Close()

	db, err := newTestClient(t, srv.URL).GetDatabase(context.Background(), "db-abc")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/databases/db-abc" {
		t.Errorf("path = %q, want /api/v1/databases/db-abc", gotPath)
	}

	// Visible (non-secret) fields decode as a typed object, not an opaque blob.
	if db.Name != "pg-restaurant-core-api-staging" {
		t.Errorf("Name = %q", db.Name)
	}
	if db.DatabaseType != "standalone-postgresql" {
		t.Errorf("DatabaseType = %q", db.DatabaseType)
	}
	if db.Image != "postgres:18-alpine" {
		t.Errorf("Image = %q", db.Image)
	}
	if db.IsPublic {
		t.Error("IsPublic = true, want false")
	}
	if db.PublicPort != 5432 {
		t.Errorf("PublicPort = %d, want 5432", db.PublicPort)
	}
	if db.SSLMode != "require" {
		t.Errorf("SSLMode = %q", db.SSLMode)
	}
	if db.EnvironmentID != 4 {
		t.Errorf("EnvironmentID = %d, want 4", db.EnvironmentID)
	}
	if db.LimitsCPUShares != 1024 {
		t.Errorf("LimitsCPUShares = %d, want 1024", db.LimitsCPUShares)
	}
	if db.LimitsMemory != "0" {
		t.Errorf("LimitsMemory = %q, want 0", db.LimitsMemory)
	}
	if db.Destination.Network != "coolify" {
		t.Errorf("Destination.Network = %q, want coolify", db.Destination.Network)
	}

	// Credential fields decode into opaque secrets: present, redacted, never in clear.
	if db.PostgresPassword.IsZero() {
		t.Error("postgres_password should decode into a non-zero Secret")
	}
	if db.InternalDBURL.IsZero() {
		t.Error("internal_db_url should decode into a non-zero Secret")
	}
	for _, s := range []secrets.Secret{db.PostgresPassword, db.InternalDBURL} {
		if s.String() != "[REDACTED]" {
			t.Errorf("credential String() = %q, want [REDACTED]", s.String())
		}
	}
	blob := db.PostgresPassword.String() + db.InternalDBURL.String() + db.Name + db.Image
	for _, leak := range []string{"REDACTED-postgres-password", "REDACTED-internal-db-url-contains-password"} {
		if strings.Contains(blob, leak) {
			t.Errorf("credential value %q leaked through a string representation", leak)
		}
	}
}

func TestGetServerResources_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"Unauthenticated."}`))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv.URL).GetServerResources(context.Background(), "srv-abc")
	if err == nil {
		t.Fatal("want error on 401")
	}
	var apiErr *coolify.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want APIError 401, got %v", err)
	}
	if strings.Contains(err.Error(), testToken) {
		t.Error("error message leaks the token")
	}
}

// cfAccessCapture is a test server that records the CF Access headers it received.
func cfAccessCapture(t *testing.T, body string) (*httptest.Server, *map[string]string) {
	t.Helper()
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen["CF-Access-Client-Id"] = r.Header.Get("CF-Access-Client-Id")
		seen["CF-Access-Client-Secret"] = r.Header.Get("CF-Access-Client-Secret")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// TestCFAccessHeadersPresentWhenConfigured asserts both CF-Access headers are sent on
// every request when the client is configured with them, and the secret reaches the wire
// only via the allowlisted Reveal() boundary.
func TestCFAccessHeadersPresentWhenConfigured(t *testing.T) {
	srv, seen := cfAccessCapture(t, "[]")

	t.Setenv("COOLIFY_API_TOKEN", testToken)
	t.Setenv("CF_ACCESS_CLIENT_SECRET", "cf-secret-DO_NOT_LEAK")
	tok, err := secrets.NewFromEnv("COOLIFY_API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	cfSec, err := secrets.NewFromEnv("CF_ACCESS_CLIENT_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	c, err := coolify.NewClient(coolify.Options{
		BaseURL:              srv.URL,
		Token:                tok,
		CFAccessClientID:     "cf-client-id.access",
		CFAccessClientSecret: cfSec,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListApplications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if (*seen)["CF-Access-Client-Id"] != "cf-client-id.access" {
		t.Errorf("CF-Access-Client-Id = %q, want cf-client-id.access", (*seen)["CF-Access-Client-Id"])
	}
	if (*seen)["CF-Access-Client-Secret"] != "cf-secret-DO_NOT_LEAK" {
		t.Errorf("CF-Access-Client-Secret header not sent correctly")
	}
}

// TestCFAccessHeadersAbsentWhenEmpty asserts no CF Access headers leak onto requests for
// a client configured without them.
func TestCFAccessHeadersAbsentWhenEmpty(t *testing.T) {
	srv, seen := cfAccessCapture(t, "[]")
	if _, err := newTestClient(t, srv.URL).ListApplications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if (*seen)["CF-Access-Client-Id"] != "" || (*seen)["CF-Access-Client-Secret"] != "" {
		t.Errorf("CF Access headers present on an unconfigured client: %+v", *seen)
	}
}

// TestNewClient_CFAccessHalfConfigured rejects a partial CF Access pair at construction.
func TestNewClient_CFAccessHalfConfigured(t *testing.T) {
	t.Setenv("COOLIFY_API_TOKEN", testToken)
	t.Setenv("CF_ACCESS_CLIENT_SECRET", "cf-secret")
	tok, err := secrets.NewFromEnv("COOLIFY_API_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	cfSec, err := secrets.NewFromEnv("CF_ACCESS_CLIENT_SECRET")
	if err != nil {
		t.Fatal(err)
	}
	cases := []coolify.Options{
		{BaseURL: "https://x", Token: tok, CFAccessClientID: "id-only"},
		{BaseURL: "https://x", Token: tok, CFAccessClientSecret: cfSec},
	}
	for i, opts := range cases {
		if _, err := coolify.NewClient(opts); err == nil {
			t.Errorf("case %d: want error on half-configured CF Access", i)
		}
	}
}

// TestOpenAPIChecksumVerifiedOnBoot asserts the pinned spec passes checksum verification
// and a tampered spec is rejected.
func TestOpenAPIChecksumVerifiedOnBoot(t *testing.T) {
	specPath := filepath.Join("..", "..", "testdata", "openapi", "coolify-v4.yaml")
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if vErr := coolify.VerifyOpenAPIChecksum(spec); vErr != nil {
		t.Fatalf("pinned spec failed checksum: %v", vErr)
	}

	// Tampering must be detected.
	tampered := append([]byte("# tampered\n"), spec...)
	if vErr := coolify.VerifyOpenAPIChecksum(tampered); vErr == nil {
		t.Error("tampered spec passed checksum verification")
	}

	// The on-disk .sha256 sidecar must match the spec (and therefore the in-code const,
	// which VerifyOpenAPIChecksum already proved equals sha256(spec)).
	sidecar, err := os.ReadFile(specPath + ".sha256")
	if err != nil {
		t.Fatal(err)
	}
	want := hex.EncodeToString(func() []byte { s := sha256.Sum256(spec); return s[:] }())
	if got := strings.Fields(string(sidecar))[0]; got != want {
		t.Errorf("sidecar hash = %s, want %s", got, want)
	}
}
