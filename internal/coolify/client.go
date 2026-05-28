// Package coolify is the low-level HTTP client for the Coolify v4 API. It is one
// of only two packages allowed to call secrets.Secret.Reveal() (the other is
// internal/secrets), and only at the HTTP Authorization boundary.
package coolify

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// openAPIChecksum is the sha256 of the pinned spec testdata/openapi/coolify-v4.yaml
// (kept in sync with its .sha256 sidecar and testdata/openapi/COMMIT_SHA). It lets
// the binary verify spec integrity at boot before trusting any endpoint. The nightly
// openapi-drift workflow watches upstream v4.x.
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
	// silent no-op (validate at the boundary).
	if (opts.CFAccessClientID != "") != !opts.CFAccessClientSecret.IsZero() {
		return nil, fmt.Errorf("coolify: CF Access requires both CFAccessClientID and CFAccessClientSecret, or neither")
	}
	rc := retryablehttp.NewClient()
	rc.RetryMax = 3
	// Bounded backoff so a 429/5xx storm can never busy-loop.
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
// Access headers applied.
func (c *Client) newRequest(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1"+path, nil)
	if err != nil {
		return nil, fmt.Errorf("coolify: build request: %w", err)
	}
	c.setAuthHeaders(req)
	return req, nil
}

// newWriteRequest builds a mutating request (POST/PATCH/DELETE) to path. A non-nil body is
// JSON-encoded. Every write carries an Idempotency-Key derived from the method, path and
// body, so a CI retry of the identical operation cannot create a duplicate resource.
func (c *Client) newWriteRequest(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var encoded []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("coolify: marshal %s %s body: %w", method, path, err)
		}
		encoded = b
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1"+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("coolify: build request: %w", err)
	}
	c.setAuthHeaders(req)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Idempotency-Key", idempotencyKey(method, path, encoded))
	return req, nil
}

// setAuthHeaders applies the Bearer token and optional CF Access headers. It is the single
// place Secret.Reveal() is called — exactly at the HTTP boundary, never earlier
// (allowlisted by internal/secrets/reveal_lint_test.go).
func (c *Client) setAuthHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.token.Reveal())
	req.Header.Set("Accept", "application/json")
	if c.cfAccessClientID != "" {
		req.Header.Set("CF-Access-Client-Id", c.cfAccessClientID)
		req.Header.Set("CF-Access-Client-Secret", c.cfAccessClientSecret.Reveal())
	}
}

// idempotencyKey is sha256(method + path + body): deterministic for an identical operation
// so retries reuse the key, distinct when the target or payload differs.
func idempotencyKey(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method + " " + path + "\n"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
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

// ListProjects fetches all projects via GET /api/v1/projects.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	req, err := c.newRequest(ctx, "/projects")
	if err != nil {
		return nil, err
	}
	var projects []Project
	if err := c.getJSON(req, &projects, "GET projects"); err != nil {
		return nil, err
	}
	return projects, nil
}

// ListEnvironments fetches a project's environments via
// GET /api/v1/projects/{uuid}/environments.
func (c *Client) ListEnvironments(ctx context.Context, projectUUID string) ([]Environment, error) {
	req, err := c.newRequest(ctx, "/projects/"+projectUUID+"/environments")
	if err != nil {
		return nil, err
	}
	var envs []Environment
	if err := c.getJSON(req, &envs, "GET environments "+projectUUID); err != nil {
		return nil, err
	}
	return envs, nil
}

// ListServers fetches all servers via GET /api/v1/servers. The resolver uses it to map a
// logical destination server name to the UUID required when creating an application.
func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	req, err := c.newRequest(ctx, "/servers")
	if err != nil {
		return nil, err
	}
	var servers []Server
	if err := c.getJSON(req, &servers, "GET servers"); err != nil {
		return nil, err
	}
	return servers, nil
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

// doJSON performs a mutating req, treating any 2xx as success. When dst is non-nil the
// response body is decoded into it; a nil dst discards the body (e.g. delete). A non-2xx
// response becomes an *APIError.
func (c *Client) doJSON(req *http.Request, dst any, label string) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("coolify: %s: %w", label, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return newAPIError(resp)
	}
	if dst == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("coolify: decode %s: %w", label, err)
	}
	return nil
}

// applicationCreateBody is the wire body for a create: the request fields plus the
// build_pack the chosen endpoint expects. The embedded BuildPack is json:"-", so the
// outer build_pack field is the only one serialised.
type applicationCreateBody struct {
	CreateApplicationRequest
	BuildPack string `json:"build_pack,omitempty"`
}

// CreateApplication creates an application, selecting the POST endpoint and body
// build_pack from req.BuildPack. It returns the new application's UUID.
func (c *Client) CreateApplication(ctx context.Context, req CreateApplicationRequest) (string, error) {
	endpoint, apiBuildPack, err := ApplicationCreateEndpoint(req.BuildPack)
	if err != nil {
		return "", err
	}
	if vErr := validateCreatable(req); vErr != nil {
		return "", vErr
	}
	body := applicationCreateBody{CreateApplicationRequest: req, BuildPack: apiBuildPack}
	httpReq, err := c.newWriteRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	var out CreateResponse
	if err := c.doJSON(httpReq, &out, "POST "+endpoint); err != nil {
		return "", err
	}
	return out.UUID, nil
}

// UpdateApplication patches the non-empty fields of req onto the application identified by
// uuid via PATCH /applications/{uuid}.
func (c *Client) UpdateApplication(ctx context.Context, uuid string, req UpdateApplicationRequest) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodPatch, "/applications/"+uuid, req)
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, "PATCH application "+uuid)
}

// DeleteApplication deletes the application identified by uuid. A 404 is treated as
// success so a repeated destroy is a no-op.
func (c *Client) DeleteApplication(ctx context.Context, uuid string) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodDelete, "/applications/"+uuid, nil)
	if err != nil {
		return err
	}
	return ignoreNotFound(c.doJSON(httpReq, nil, "DELETE application "+uuid))
}

// CreateProject creates a project via POST /projects and returns its UUID.
func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (string, error) {
	httpReq, err := c.newWriteRequest(ctx, http.MethodPost, "/projects", req)
	if err != nil {
		return "", err
	}
	var out CreateResponse
	if err := c.doJSON(httpReq, &out, "POST projects"); err != nil {
		return "", err
	}
	return out.UUID, nil
}

// DeleteProject deletes the project identified by uuid. A 404 is treated as success.
func (c *Client) DeleteProject(ctx context.Context, uuid string) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodDelete, "/projects/"+uuid, nil)
	if err != nil {
		return err
	}
	return ignoreNotFound(c.doJSON(httpReq, nil, "DELETE project "+uuid))
}

// CreateEnvironment creates an environment in the project identified by projectUUID via
// POST /projects/{uuid}/environments and returns its UUID.
func (c *Client) CreateEnvironment(ctx context.Context, projectUUID string, req CreateEnvironmentRequest) (string, error) {
	httpReq, err := c.newWriteRequest(ctx, http.MethodPost, "/projects/"+projectUUID+"/environments", req)
	if err != nil {
		return "", err
	}
	var out CreateResponse
	if err := c.doJSON(httpReq, &out, "POST environments "+projectUUID); err != nil {
		return "", err
	}
	return out.UUID, nil
}

// DeleteEnvironment deletes an environment by name (or UUID) from the project identified
// by projectUUID. A 404 is treated as success.
func (c *Client) DeleteEnvironment(ctx context.Context, projectUUID, envNameOrUUID string) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodDelete, "/projects/"+projectUUID+"/environments/"+envNameOrUUID, nil)
	if err != nil {
		return err
	}
	return ignoreNotFound(c.doJSON(httpReq, nil, "DELETE environment "+envNameOrUUID))
}

// ignoreNotFound maps a 404 *APIError to nil so a delete of an already-absent resource is
// a silent no-op (idempotent destroy).
func ignoreNotFound(err error) error {
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
		return nil
	}
	return err
}

// OpenAPIChecksum returns the pinned spec sha256, recorded in the state cache so a later
// run can tell whether it planned against the same API contract.
func OpenAPIChecksum() string { return openAPIChecksum }

// VerifyOpenAPIChecksum reports whether spec matches the pinned sha256.
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
