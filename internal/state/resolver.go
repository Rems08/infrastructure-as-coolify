package state

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	ListServices(ctx context.Context) ([]coolify.Service, error)
	GetServerResources(ctx context.Context, serverUUID string) ([]coolify.ServerResource, error)
}

// Resolve builds the logical-key → identifier map from live Coolify state using only
// documented v4 endpoints (GET /projects, /projects/{uuid}/environments, /servers,
// /applications, /services). The mapped value is whatever the API uses to address the
// resource: a UUID for projects, servers, applications and services; the name for
// environments (which have no UUID and are addressed by environment_name_or_uuid). It
// joins each application's and service's environment_id back to its environment and
// project names.
//
// Databases are resolved via GET /servers/{uuid}/resources (a typed endpoint), because
// the dedicated listing endpoints are placeholders in the pinned spec
// (coollabsio/coolify#10449). That response carries no environment_id, so databases are
// keyed by name alone (see resolveDatabases).
func Resolve(ctx context.Context, client resolverClient) (Map, error) {
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve: list projects: %w", err)
	}
	m := make(Map)
	for _, p := range projects {
		m[ResourceKey{Kind: resource.KindProject, Name: p.Name}] = p.UUID
	}

	envByID := make(map[int]envRef)
	for _, p := range projects {
		envs, eErr := client.ListEnvironments(ctx, p.UUID)
		if eErr != nil {
			return nil, fmt.Errorf("resolve: list environments for project %q: %w", p.Name, eErr)
		}
		for _, e := range envs {
			envByID[e.ID] = envRef{name: e.Name, project: p.Name}
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
		mapChild(m, envByID, resource.KindApplication, a.Name, a.UUID, a.EnvironmentID)
	}

	services, err := client.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve: list services: %w", err)
	}
	for _, s := range services {
		mapChild(m, envByID, resource.KindService, s.Name, s.UUID, s.EnvironmentID)
	}

	if err := resolveDatabases(ctx, client, servers, m); err != nil {
		return nil, err
	}
	return m, nil
}

// resolveDatabases keys each standalone database found across the servers by its name.
// The /servers/{uuid}/resources response carries no project or environment, so databases
// are addressed by name alone — Coolify enforces unique database names per server, and the
// Beenaire convention suffixes environments explicitly (e.g. -staging). The "standalone-"
// type prefix (observed runtime 2026-05-29: standalone-postgresql, standalone-redis, ...)
// distinguishes a database from an application or service in the homogeneous array.
func resolveDatabases(ctx context.Context, c resolverClient, servers []coolify.Server, m Map) error {
	var total, filtered int
	for _, srv := range servers {
		resources, err := c.GetServerResources(ctx, srv.UUID)
		if err != nil {
			return fmt.Errorf("resolve: get resources for server %q: %w", srv.UUID, err)
		}
		total += len(resources)
		for _, r := range resources {
			if !strings.HasPrefix(r.Type, "standalone-") {
				continue
			}
			filtered++
			m[ResourceKey{Kind: resource.KindDatabase, Name: r.Name}] = r.UUID
		}
	}
	slog.InfoContext(ctx, "resolved databases",
		"resolver.databases.servers_scanned", len(servers),
		"resolver.databases.resources_total", total,
		"resolver.databases.standalone_filtered", filtered,
		"resolver.databases.resolved", filtered,
	)
	return nil
}

// envRef is the (environment name, project name) pair an application or service is keyed
// under. The project is captured while enumerating each project's environments, not read
// from the environment payload: GET /projects/{uuid}/environments does not populate
// project_id at runtime, so deriving it there would drop every child (see mapChild).
type envRef struct {
	name    string
	project string
}

// mapChild keys an application or service by its (project, environment, name) coordinates,
// resolved from its environment_id. An item in an environment we couldn't enumerate is
// skipped rather than mis-keyed.
func mapChild(m Map, envByID map[int]envRef, kind, name, uuid string, envID int) {
	ref, ok := envByID[envID]
	if !ok {
		return
	}
	m[ResourceKey{Project: ref.project, Environment: ref.name, Kind: kind, Name: name}] = uuid
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
