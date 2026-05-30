package coolify

import (
	"context"
	"net/http"
)

// ListApplicationEnvs fetches an application's env vars via GET /applications/{uuid}/envs.
// The env-var wire shape is identical to a service's, so ServiceEnvVar is reused as the
// carrier; the returned values are the API's stored values (Secret is never populated from
// a response).
func (c *Client) ListApplicationEnvs(ctx context.Context, appUUID string) ([]ServiceEnvVar, error) {
	if err := requireUUID(appUUID); err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, "/applications/"+appUUID+"/envs")
	if err != nil {
		return nil, err
	}
	var envs []ServiceEnvVar
	if err := c.getJSON(req, &envs, "GET application envs "+appUUID); err != nil {
		return nil, err
	}
	return envs, nil
}

// BulkUpdateApplicationEnvs replaces an application's env vars in one call via
// PATCH /applications/{uuid}/envs/bulk. The application API has no single-env POST, so a
// create and an update both go through this bulk endpoint. A Secret is revealed here, at
// the HTTP boundary, never by the caller.
func (c *Client) BulkUpdateApplicationEnvs(ctx context.Context, appUUID string, envs []ServiceEnvVar) error {
	if err := requireUUID(appUUID); err != nil {
		return err
	}
	body := bulkEnvBody{Data: make([]envWire, 0, len(envs))}
	for _, e := range envs {
		body.Data = append(body.Data, e.wire())
	}
	httpReq, err := c.newWriteRequest(ctx, http.MethodPatch, "/applications/"+appUUID+"/envs/bulk", body)
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, "PATCH application envs bulk "+appUUID)
}

// DeleteApplicationEnv deletes one env var via DELETE /applications/{uuid}/envs/{env_uuid}.
// A 404 is treated as success so a repeated delete is a no-op.
func (c *Client) DeleteApplicationEnv(ctx context.Context, appUUID, envUUID string) error {
	if err := requireUUID(appUUID); err != nil {
		return err
	}
	httpReq, err := c.newWriteRequest(ctx, http.MethodDelete, "/applications/"+appUUID+"/envs/"+envUUID, nil)
	if err != nil {
		return err
	}
	return ignoreNotFound(c.doJSON(httpReq, nil, "DELETE application env "+appUUID))
}
