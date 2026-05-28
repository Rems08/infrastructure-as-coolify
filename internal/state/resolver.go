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

// Has reports whether key was resolved.
func (m Map) Has(k ResourceKey) bool {
	_, ok := m[k]
	return ok
}

// KindServer is the resolver coordinate for a Coolify server. A server is not a
// user-declarable resource (iac-coolify never creates one); the resolver maps its name to
// the UUID an application-create body requires.
const KindServer = "Server"

// resolverClient is the subset of the Coolify client the resolver needs (accept
// interfaces, return structs).
type resolverClient interface {
	ListProjects(ctx context.Context) ([]coolify.Project, error)
	ListEnvironments(ctx context.Context, projectUUID string) ([]coolify.Environment, error)
	ListServers(ctx context.Context) ([]coolify.Server, error)
	ListApplications(ctx context.Context) ([]coolify.Application, error)
}

// Resolve builds the logical-key → identifier map from live Coolify state using only
// documented v4 endpoints (GET /projects, /projects/{uuid}/environments, /servers,
// /applications). The mapped value is whatever the API uses to address the resource: a
// UUID for projects, servers and applications; the name for environments (which have no
// UUID and are addressed by environment_name_or_uuid). It joins each application's
// environment_id back to its environment and project names.
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
	m := make(Map)
	projectByID := make(map[int]coolify.Project, len(projects))
	for _, p := range projects {
		projectByID[p.ID] = p
		m[ResourceKey{Kind: resource.KindProject, Name: p.Name}] = p.UUID
	}

	envByID := make(map[int]coolify.Environment)
	for _, p := range projects {
		envs, eErr := client.ListEnvironments(ctx, p.UUID)
		if eErr != nil {
			return nil, fmt.Errorf("resolve: list environments for project %q: %w", p.Name, eErr)
		}
		for _, e := range envs {
			envByID[e.ID] = e
			m[ResourceKey{Project: p.Name, Kind: resource.KindEnvironment, Name: e.Name}] = e.Name
		}
	}

	servers, err := client.ListServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve: list servers: %w", err)
	}
	for _, s := range servers {
		m[ResourceKey{Kind: KindServer, Name: s.Name}] = s.UUID
	}

	apps, err := client.ListApplications(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve: list applications: %w", err)
	}
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

// Save writes the resolved map to an opt-in JSON cache at path (used by --state-cache).
// State.MarshalJSON guarantees no Secret is ever serialised.
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
