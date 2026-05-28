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
	baseURL string
	token   secrets.Secret
	http    *http.Client
}

// Options configures a Client. Token must be built via secrets.NewFromEnv.
type Options struct {
	BaseURL string         // e.g. https://coolify.beenaire.com
	Token   secrets.Secret // Bearer Sanctum token
	Timeout time.Duration  // per-request timeout; defaults to 30s
}

// NewClient validates opts and returns a ready Client. It never panics.
func NewClient(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, fmt.Errorf("coolify: base URL required")
	}
	if opts.Token.IsZero() {
		return nil, fmt.Errorf(`coolify: token required (build via secrets.NewFromEnv("COOLIFY_API_TOKEN"))`)
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
		baseURL: opts.BaseURL,
		token:   opts.Token,
		http:    rc.StandardClient(),
	}, nil
}

// ListApplications fetches all applications via GET /api/v1/applications.
func (c *Client) ListApplications(ctx context.Context) ([]Application, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/applications", nil)
	if err != nil {
		return nil, fmt.Errorf("coolify: build request: %w", err)
	}
	// The only Reveal() call site outside internal/secrets — exactly at the HTTP
	// boundary, never earlier. Allowlisted by internal/secrets/reveal_lint_test.go.
	req.Header.Set("Authorization", "Bearer "+c.token.Reveal())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		// err cannot contain the token: c.token is a Secret, so it never reaches
		// any string/format without an (allowlist-blocked) Reveal() call.
		return nil, fmt.Errorf("coolify: GET applications: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, newAPIError(resp)
	}

	var apps []Application
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		return nil, fmt.Errorf("coolify: decode applications: %w", err)
	}
	return apps, nil
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
