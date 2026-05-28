// Package coolify is the low-level HTTP client for the Coolify v4 API. It is one
// of only two packages allowed to call secrets.Secret.Reveal() (the other is
// internal/secrets), and only at the HTTP Authorization boundary.
package coolify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// openAPIChecksum is the sha256 of the pinned spec testdata/openapi/coolify-v4.yaml
// (kept in sync with its .sha256 sidecar and testdata/openapi/COMMIT_SHA). It lets
// the binary verify spec integrity at boot before trusting any endpoint (C-S7.3,
// threat-model T-S7.3). The nightly openapi-drift workflow watches upstream v4.x.
const openAPIChecksum = "e98fa2b00ce84fb9eae326999c89e6b9d87e96c65528ba7e1da754010cb44413"

// maxErrorBodyBytes caps how much of a non-2xx response body is echoed in an error,
// to keep messages bounded.
const maxErrorBodyBytes = 512

// Client talks to a Coolify v4 instance over HTTPS with a Bearer token.
type Client struct {
	baseURL              string
	token                secrets.Secret
	cfAccessClientID     string
	cfAccessClientSecret secrets.Secret
	http                 *http.Client
}

// Options configures a Client. Token must be built via secrets.NewFromEnv.
type Options struct {
	BaseURL string         // e.g. https://coolify.beenaire.com
	Token   secrets.Secret // Bearer Sanctum token
	// CFAccessClientID and CFAccessClientSecret authenticate the request to a
	// Cloudflare Access zero-trust gateway sitting in front of Coolify. The ID is a
	// public service-token identifier; the secret is typed Secret and revealed only at
	// the HTTP header boundary. Both empty (the default) disables CF Access headers.
	CFAccessClientID     string
	CFAccessClientSecret secrets.Secret
	Timeout              time.Duration // per-request timeout; defaults to 30s
}

// NewClient validates opts and returns a ready Client. It never panics.
func NewClient(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, fmt.Errorf("coolify: base URL required")
	}
	if opts.Token.IsZero() {
		return nil, fmt.Errorf(`coolify: token required (build via secrets.NewFromEnv("COOLIFY_API_TOKEN"))`)
	}
	// CF Access is all-or-nothing: a half-configured pair is a config bug, not a
	// silent no-op (validate at the boundary, cf. CLAUDE.md §5).
	if (opts.CFAccessClientID != "") != !opts.CFAccessClientSecret.IsZero() {
		return nil, fmt.Errorf("coolify: CF Access requires both CFAccessClientID and CFAccessClientSecret, or neither")
	}
	rc := retryablehttp.NewClient()
	rc.RetryMax = 3
	// Bounded backoff so a 429/5xx storm can never busy-loop (threat-model T-S1.8).
	// retryablehttp's default backoff already honours the Retry-After header.
	rc.RetryWaitMin = 1 * time.Second
	rc.RetryWaitMax = 30 * time.Second
	rc.Logger = nil // no stdout/stderr noise from the métier
	// Surface the final response (e.g. a persistent 429) instead of an opaque
	// "giving up after N attempts" error, so callers can inspect the status code.
	rc.ErrorHandler = retryablehttp.PassthroughErrorHandler
	rc.HTTPClient.Timeout = orDefault(opts.Timeout, 30*time.Second)
	return &Client{
		baseURL:              opts.BaseURL,
		token:                opts.Token,
		cfAccessClientID:     opts.CFAccessClientID,
		cfAccessClientSecret: opts.CFAccessClientSecret,
		http:                 rc.StandardClient(),
	}, nil
}

// newRequest builds a GET request to path (relative to /api/v1) with the auth and CF
// Access headers applied. It is the single place Secret.Reveal() is called — exactly at
// the HTTP boundary, never earlier (allowlisted by internal/secrets/reveal_lint_test.go).
func (c *Client) newRequest(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("coolify: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token.Reveal())
	req.Header.Set("Accept", "application/json")
	if c.cfAccessClientID != "" {
		req.Header.Set("CF-Access-Client-Id", c.cfAccessClientID)
		req.Header.Set("CF-Access-Client-Secret", c.cfAccessClientSecret.Reveal())
	}
	return req, nil
}

// getJSON performs req and decodes a 2xx JSON body into dst. label names the call in
// errors. A non-2xx response becomes an *APIError. The token never reaches an error:
// it is a Secret, unreachable by any format verb without an allowlisted Reveal().
func (c *Client) getJSON(req *http.Request, dst any, label string) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("coolify: %s: %w", label, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return newAPIError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("coolify: decode %s: %w", label, err)
	}
	return nil
}

// ListApplications fetches all applications via GET /api/v1/applications.
func (c *Client) ListApplications(ctx context.Context) ([]Application, error) {
	req, err := c.newRequest(ctx, "/applications")
	if err != nil {
		return nil, err
	}
	var apps []Application
	if err := c.getJSON(req, &apps, "GET applications"); err != nil {
		return nil, err
	}
	return apps, nil
}

// GetApplication fetches a single application by UUID via GET /api/v1/applications/{uuid}.
// It is the documented endpoint used to read remote state for the plan diff.
func (c *Client) GetApplication(ctx context.Context, uuid string) (Application, error) {
	var app Application
	req, err := c.newRequest(ctx, "/applications/"+uuid)
	if err != nil {
		return app, err
	}
	if err := c.getJSON(req, &app, "GET application "+uuid); err != nil {
		return app, err
	}
	return app, nil
}

// VerifyOpenAPIChecksum reports whether spec matches the pinned sha256 (C-S7.3).
// Wiring it into command boot (refuse to run on mismatch) lands when the spec is
// loaded for endpoint validation in Wave 2.
func VerifyOpenAPIChecksum(spec []byte) error {
	sum := sha256.Sum256(spec)
	got := hex.EncodeToString(sum[:])
	if got != openAPIChecksum {
		return fmt.Errorf("coolify: OpenAPI checksum mismatch: got %s, want %s", got, openAPIChecksum)
	}
	return nil
}

// APIError describes a non-2xx Coolify response.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("coolify: API error %s: %s", e.Status, e.Body)
}

func newAPIError(resp *http.Response) *APIError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	return &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       string(body),
	}
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}
