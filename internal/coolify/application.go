package coolify

import (
	"context"
	"fmt"
)

// requireUUID rejects an empty resource UUID before it can be interpolated into a path,
// where it would silently produce a request to the collection endpoint instead.
func requireUUID(uuid string) error {
	if uuid == "" {
		return fmt.Errorf("coolify: application uuid required")
	}
	return nil
}

// StartApplication starts the application identified by uuid.
func (c *Client) StartApplication(ctx context.Context, uuid string) error {
	return c.applicationLifecycle(ctx, uuid, "start")
}

// StopApplication stops the application identified by uuid.
func (c *Client) StopApplication(ctx context.Context, uuid string) error {
	return c.applicationLifecycle(ctx, uuid, "stop")
}

// RestartApplication restarts the application identified by uuid.
func (c *Client) RestartApplication(ctx context.Context, uuid string) error {
	return c.applicationLifecycle(ctx, uuid, "restart")
}

// applicationLifecycle drives an application start/stop/restart. Coolify exposes these as
// GET endpoints (unlike the service lifecycle, which is POST), so the request carries no
// body and no Idempotency-Key — a GET is idempotent by contract.
func (c *Client) applicationLifecycle(ctx context.Context, uuid, action string) error {
	if err := requireUUID(uuid); err != nil {
		return err
	}
	req, err := c.newRequest(ctx, "/applications/"+uuid+"/"+action)
	if err != nil {
		return err
	}
	return c.doJSON(req, nil, action+" application "+uuid)
}
