package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// ResourceKey identifies a managed resource by its logical coordinates, never by UUID
// (G-OBS-8). It is the stable key the resolver maps to a live Coolify UUID.
type ResourceKey struct {
	Project     string
	Environment string
	Kind        string
	Name        string
}

// String renders the key for the on-disk cache. Logical names must not contain "/".
func (k ResourceKey) String() string {
	return strings.Join([]string{k.Project, k.Environment, k.Kind, k.Name}, "/")
}

// Map resolves logical resource keys to live Coolify UUIDs.
type Map map[ResourceKey]string

// Lookup returns the UUID for key, reporting whether it was resolved.
func (m Map) Lookup(k ResourceKey) (string, bool) {
	u, ok := m[k]
	return u, ok
}

// resolverClient is the subset of the Coolify client the resolver needs (accept
// interfaces, return structs).
type resolverClient interface {
	ListProjects(ctx context.Context) ([]coolify.Project, error)
	ListEnvironments(ctx context.Context, projectUUID string) ([]coolify.Environment, error)
	ListApplications(ctx context.Context) ([]coolify.Application, error)
}

// Resolve builds the logical-key → UUID map from live Coolify state using only documented
// v4 endpoints (GET /projects, /projects/{uuid}/environments, /applications). It joins
// each application's environment_id back to its environment and project names.
//
// Databases and Services are intentionally out of scope: their list responses are
// undocumented placeholders in the pinned OpenAPI spec (escalation #2), so resolving them
// would mean inventing the response shape. Live resolution for those kinds lands once the
// upstream spec documents them.
func Resolve(ctx context.Context, client resolverClient) (Map, error) {
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve: list projects: %w", err)
	}
	projectByID := make(map[int]coolify.Project, len(projects))
	for _, p := range projects {
		projectByID[p.ID] = p
	}

	envByID := make(map[int]coolify.Environment)
	for _, p := range projects {
		envs, eErr := client.ListEnvironments(ctx, p.UUID)
		if eErr != nil {
			return nil, fmt.Errorf("resolve: list environments for project %q: %w", p.Name, eErr)
		}
		for _, e := range envs {
			envByID[e.ID] = e
		}
	}

	apps, err := client.ListApplications(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve: list applications: %w", err)
	}
	m := make(Map, len(apps))
	for _, a := range apps {
		env, ok := envByID[a.EnvironmentID]
		if !ok {
			continue // app in an environment we couldn't enumerate; skip rather than mis-key
		}
		project, ok := projectByID[env.ProjectID]
		if !ok {
			continue
		}
		m[ResourceKey{
			Project:     project.Name,
			Environment: env.Name,
			Kind:        resource.KindApplication,
			Name:        a.Name,
		}] = a.UUID
	}
	return m, nil
}

// Save writes the resolved map to an opt-in JSON cache at path (ADR-2, --state-cache).
// State.MarshalJSON guarantees no Secret is ever serialised (C-L0.4 ratchet).
func (m Map) Save(path, openAPIHash string, at time.Time) error {
	st := &State{
		UUIDs:       make(map[string]string, len(m)),
		ResolvedAt:  at.UTC(),
		OpenAPIHash: openAPIHash,
	}
	for k, uuid := range m {
		st.UUIDs[k.String()] = uuid
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("state: marshal cache: %w", err)
	}
	if dir := filepath.Dir(path); dir != "" {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			return fmt.Errorf("state: mkdir cache dir: %w", mkErr)
		}
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("state: write cache: %w", err)
	}
	return nil
}
