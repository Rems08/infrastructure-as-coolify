package coolify

import (
	"context"
	"encoding/base64"
	"net/http"
)

// ListServices fetches all services via GET /api/v1/services. The resolver uses it to map
// a logical (project, environment, service name) to its UUID.
func (c *Client) ListServices(ctx context.Context) ([]Service, error) {
	req, err := c.newRequest(ctx, "/services")
	if err != nil {
		return nil, err
	}
	var services []Service
	if err := c.getJSON(req, &services, "GET services"); err != nil {
		return nil, err
	}
	return services, nil
}

// serviceCreateBody is the wire body for a create: the request fields plus the
// base64-encoded compose content the API expects under docker_compose_raw.
type serviceCreateBody struct {
	CreateServiceRequest
	DockerComposeRawB64 string `json:"docker_compose_raw,omitempty"`
}

// CreateService creates a service via POST /services and returns its UUID. A compose
// stack (DockerComposeRaw) is base64-encoded into the request body; a one-click template
// (Type) is passed through unchanged.
func (c *Client) CreateService(ctx context.Context, req CreateServiceRequest) (string, error) {
	body := serviceCreateBody{CreateServiceRequest: req}
	if req.DockerComposeRaw != "" {
		body.DockerComposeRawB64 = base64.StdEncoding.EncodeToString([]byte(req.DockerComposeRaw))
	}
	httpReq, err := c.newWriteRequest(ctx, http.MethodPost, "/services", body)
	if err != nil {
		return "", err
	}
	var out CreateResponse
	if err := c.doJSON(httpReq, &out, "POST services"); err != nil {
		return "", err
	}
	return out.UUID, nil
}

// serviceUpdateBody is the wire body for a PATCH: the request fields plus the
// base64-encoded compose content.
type serviceUpdateBody struct {
	UpdateServiceRequest
	DockerComposeRawB64 string `json:"docker_compose_raw,omitempty"`
}

// UpdateService patches the non-empty fields of req onto the service identified by uuid
// via PATCH /services/{uuid}.
func (c *Client) UpdateService(ctx context.Context, uuid string, req UpdateServiceRequest) error {
	body := serviceUpdateBody{UpdateServiceRequest: req}
	if req.DockerComposeRaw != "" {
		body.DockerComposeRawB64 = base64.StdEncoding.EncodeToString([]byte(req.DockerComposeRaw))
	}
	httpReq, err := c.newWriteRequest(ctx, http.MethodPatch, "/services/"+uuid, body)
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, "PATCH service "+uuid)
}

// DeleteService deletes the service identified by uuid. A 404 is treated as success so a
// repeated destroy is a no-op.
func (c *Client) DeleteService(ctx context.Context, uuid string) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodDelete, "/services/"+uuid, nil)
	if err != nil {
		return err
	}
	return ignoreNotFound(c.doJSON(httpReq, nil, "DELETE service "+uuid))
}

// StartService starts the service identified by uuid. The endpoint also accepts POST,
// which is used here so the request carries an Idempotency-Key.
func (c *Client) StartService(ctx context.Context, uuid string) error {
	return c.serviceLifecycle(ctx, uuid, "start")
}

// StopService stops the service identified by uuid.
func (c *Client) StopService(ctx context.Context, uuid string) error {
	return c.serviceLifecycle(ctx, uuid, "stop")
}

// RestartService restarts the service identified by uuid.
func (c *Client) RestartService(ctx context.Context, uuid string) error {
	return c.serviceLifecycle(ctx, uuid, "restart")
}

func (c *Client) serviceLifecycle(ctx context.Context, uuid, action string) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodPost, "/services/"+uuid+"/"+action, nil)
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, action+" service "+uuid)
}

// envWire is the request shape for a single service env var: only key and value reach the
// API. A Secret is revealed here, at the HTTP boundary, never by the caller.
type envWire struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func (e ServiceEnvVar) wire() envWire {
	v := e.Value
	if !e.Secret.IsZero() {
		v = e.Secret.Reveal()
	}
	return envWire{Key: e.Key, Value: v}
}

// ListServiceEnvs fetches a service's env vars via GET /services/{uuid}/envs. The returned
// values are the API's stored values; Secret is never populated from a response.
func (c *Client) ListServiceEnvs(ctx context.Context, serviceUUID string) ([]ServiceEnvVar, error) {
	req, err := c.newRequest(ctx, "/services/"+serviceUUID+"/envs")
	if err != nil {
		return nil, err
	}
	var envs []ServiceEnvVar
	if err := c.getJSON(req, &envs, "GET service envs "+serviceUUID); err != nil {
		return nil, err
	}
	return envs, nil
}

// CreateServiceEnv creates one env var via POST /services/{uuid}/envs.
func (c *Client) CreateServiceEnv(ctx context.Context, serviceUUID string, env ServiceEnvVar) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodPost, "/services/"+serviceUUID+"/envs", env.wire())
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, "POST service env "+serviceUUID)
}

// UpdateServiceEnv updates one env var via PATCH /services/{uuid}/envs. The API identifies
// the variable by its key (carried in the body), not by a path UUID.
func (c *Client) UpdateServiceEnv(ctx context.Context, serviceUUID string, env ServiceEnvVar) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodPatch, "/services/"+serviceUUID+"/envs", env.wire())
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, "PATCH service env "+serviceUUID)
}

// DeleteServiceEnv deletes one env var via DELETE /services/{uuid}/envs/{env_uuid}. A 404
// is treated as success.
func (c *Client) DeleteServiceEnv(ctx context.Context, serviceUUID, envUUID string) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodDelete, "/services/"+serviceUUID+"/envs/"+envUUID, nil)
	if err != nil {
		return err
	}
	return ignoreNotFound(c.doJSON(httpReq, nil, "DELETE service env "+serviceUUID))
}

// bulkEnvBody is the wire body for the bulk env update: {"data": [{key, value}, ...]}.
type bulkEnvBody struct {
	Data []envWire `json:"data"`
}

// BulkUpdateServiceEnvs replaces a service's env vars in one call via
// PATCH /services/{uuid}/envs/bulk.
func (c *Client) BulkUpdateServiceEnvs(ctx context.Context, serviceUUID string, envs []ServiceEnvVar) error {
	body := bulkEnvBody{Data: make([]envWire, 0, len(envs))}
	for _, e := range envs {
		body.Data = append(body.Data, e.wire())
	}
	httpReq, err := c.newWriteRequest(ctx, http.MethodPatch, "/services/"+serviceUUID+"/envs/bulk", body)
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, "PATCH service envs bulk "+serviceUUID)
}
