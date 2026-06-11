package importer

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

// Client is the subset of the Coolify client the importer reads (accept interfaces). It
// enumerates per server, resolves environments per project, and fetches each resource's
// detail. It needs no write or reveal capability.
type Client interface {
	ListServers(ctx context.Context) ([]coolify.Server, error)
	GetServerResources(ctx context.Context, serverUUID string) ([]coolify.ServerResource, error)
	ListProjects(ctx context.Context) ([]coolify.Project, error)
	ListEnvironments(ctx context.Context, projectUUID string) ([]coolify.Environment, error)
	GetApplication(ctx context.Context, uuid string) (coolify.Application, error)
	GetDatabase(ctx context.Context, uuid string) (coolify.Database, error)
	ListApplicationEnvs(ctx context.Context, appUUID string) ([]coolify.ServiceEnvVar, error)
}

// Options configure an import run.
type Options struct {
	Dir            string   // target directory the manifests are scaffolded under
	DefaultNetwork string   // network written when the live payload does not expose one
	EnvFilter      []string // when non-empty, only these environments are imported
	Force          bool     // overwrite existing manifests instead of refusing
	APIURL         string   // Coolify URL recorded in the scaffolded root manifest
}

// AppResult records one imported application's name, environment and whether it round-trips
// to a valid manifest. A partial application carries the validation error explaining what an
// operator must complete by hand (typically the git repository the API does not expose).
type AppResult struct {
	Name        string
	Environment string
	Complete    bool
	Reason      string
}

// Report summarises an import: what was written, what was skipped, and which env vars the
// operator must populate. It mirrors the resolver's no-silent-drop discipline by counting
// every resource that could not be mapped.
type Report struct {
	Applications    []AppResult
	Databases       []string
	ServicesSkipped []string
	EnvKeys         []string
	PasswordEnvs    []string
	Dropped         int
}

const (
	kindApplication = "application"
	kindService     = "service"
)

// discovered is one resource found on a server, before its detail is fetched. The server
// name is captured here because only the per-server enumeration reveals it.
type discovered struct {
	kind   string
	uuid   string
	name   string
	server string
}

// Run imports every mapped application and database from the live instance into opts.Dir.
// It scaffolds the root manifest and the environment tree, writes a ${env:} reference for
// every secret rather than its value, and refuses to overwrite existing files unless
// opts.Force is set. The returned Report describes the outcome even on a write error.
func Run(ctx context.Context, client Client, opts Options) (Report, error) {
	envByID, err := buildEnvIndex(ctx, client)
	if err != nil {
		return Report{}, err
	}
	found, err := enumerate(ctx, client)
	if err != nil {
		return Report{}, err
	}

	var rep Report
	files := []plannedFile{planRoot(opts.Dir, opts.APIURL)}
	for _, d := range found {
		pf, ok, cErr := collect(ctx, client, d, opts, envByID, &rep)
		if cErr != nil {
			return rep, cErr
		}
		if ok {
			files = append(files, pf)
		}
	}
	dedupeReport(&rep)

	if err := commitFiles(files, opts.Force); err != nil {
		return rep, err
	}
	return rep, nil
}

// collect maps one discovered resource into a planned file, updating rep. A service is
// recorded as skipped; an application or database whose environment_id was not enumerated is
// counted as dropped (logged, never silent). ok is false when nothing is to be written.
func collect(ctx context.Context, client Client, d discovered, opts Options, envByID map[int]envRef, rep *Report) (plannedFile, bool, error) {
	switch d.kind {
	case kindService:
		rep.ServicesSkipped = append(rep.ServicesSkipped, d.name)
		return plannedFile{}, false, nil
	case kindApplication:
		return collectApplication(ctx, client, d, opts, envByID, rep)
	default:
		return collectDatabase(ctx, client, d, opts, envByID, rep)
	}
}

func collectApplication(ctx context.Context, client Client, d discovered, opts Options, envByID map[int]envRef, rep *Report) (plannedFile, bool, error) {
	app, err := client.GetApplication(ctx, d.uuid)
	if err != nil {
		return plannedFile{}, false, err
	}
	env, ok := envByID[app.EnvironmentID]
	if !ok {
		drop(ctx, rep, kindApplication, d.name, app.EnvironmentID)
		return plannedFile{}, false, nil
	}
	if !matchesEnv(opts.EnvFilter, env.name) {
		return plannedFile{}, false, nil
	}
	envs, err := client.ListApplicationEnvs(ctx, d.uuid)
	if err != nil {
		return plannedFile{}, false, err
	}
	mapped, keys := mapApplication(app, d.server, networkOrDefault(app.Destination.Network, opts.DefaultNetwork), env, envs)
	res := AppResult{Name: mapped.Metadata.Name, Environment: env.name, Complete: true}
	if vErr := mapped.Validate(); vErr != nil {
		res.Complete = false
		res.Reason = vErr.Error()
	}
	rep.Applications = append(rep.Applications, res)
	rep.EnvKeys = append(rep.EnvKeys, keys...)
	return planApplication(opts.Dir, mapped), true, nil
}

func collectDatabase(ctx context.Context, client Client, d discovered, opts Options, envByID map[int]envRef, rep *Report) (plannedFile, bool, error) {
	db, err := client.GetDatabase(ctx, d.uuid)
	if err != nil {
		return plannedFile{}, false, err
	}
	env, ok := envByID[db.EnvironmentID]
	if !ok {
		drop(ctx, rep, "database", d.name, db.EnvironmentID)
		return plannedFile{}, false, nil
	}
	if !matchesEnv(opts.EnvFilter, env.name) {
		return plannedFile{}, false, nil
	}
	mapped, passwordEnv, err := mapDatabase(db, d.server, networkOrDefault(db.Destination.Network, opts.DefaultNetwork), env)
	if err != nil {
		return plannedFile{}, false, err
	}
	rep.Databases = append(rep.Databases, mapped.Metadata.Name)
	rep.PasswordEnvs = append(rep.PasswordEnvs, passwordEnv)
	return planDatabase(opts.Dir, mapped), true, nil
}

// networkOrDefault prefers the network reported by the live destination payload, so an
// imported manifest plans clean even when the --default-network flag does not match the
// instance. The fallback covers payloads that omit the nested destination object.
func networkOrDefault(live, fallback string) string {
	if live != "" {
		return live
	}
	return fallback
}

// buildEnvIndex maps each environment id to its (project, environment) names by enumerating
// every project's environments. The project is captured here, never read from the
// environment payload (the API leaves project_id zero), mirroring the resolver.
func buildEnvIndex(ctx context.Context, client Client) (map[int]envRef, error) {
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("import: list projects: %w", err)
	}
	index := make(map[int]envRef)
	for _, p := range projects {
		envs, eErr := client.ListEnvironments(ctx, p.UUID)
		if eErr != nil {
			return nil, fmt.Errorf("import: list environments for project %q: %w", p.Name, eErr)
		}
		for _, e := range envs {
			index[e.ID] = envRef{project: p.Name, name: e.Name}
		}
	}
	return index, nil
}

// enumerate walks every server's resources and classifies each by its type discriminator,
// capturing the hosting server name. The server is otherwise unavailable: GET /applications
// does not report it, so the per-server endpoint is the only source of Destination.Server.
func enumerate(ctx context.Context, client Client) ([]discovered, error) {
	servers, err := client.ListServers(ctx)
	if err != nil {
		return nil, fmt.Errorf("import: list servers: %w", err)
	}
	var found []discovered
	for _, srv := range servers {
		resources, rErr := client.GetServerResources(ctx, srv.UUID)
		if rErr != nil {
			return nil, fmt.Errorf("import: get resources for server %q: %w", srv.Name, rErr)
		}
		for _, r := range resources {
			if kind, ok := classify(r.Type); ok {
				found = append(found, discovered{kind: kind, uuid: r.UUID, name: r.Name, server: srv.Name})
			} else {
				slog.WarnContext(ctx, "import: skipping resource of unrecognised type",
					"import.resource.name", r.Name, "import.resource.type", r.Type)
			}
		}
	}
	return found, nil
}

// classify maps a server-resource type to the importer kind, reporting ok=false for a type
// the importer does not handle (so the caller logs it rather than guessing).
func classify(resourceType string) (string, bool) {
	switch {
	case resourceType == kindApplication:
		return kindApplication, true
	case resourceType == kindService:
		return kindService, true
	case strings.HasPrefix(resourceType, "standalone-"):
		return "database", true
	default:
		return "", false
	}
}

// drop records a resource that could not be keyed to an environment, logging it so a future
// API divergence surfaces at once instead of leaving the import silently incomplete.
func drop(ctx context.Context, rep *Report, kind, name string, envID int) {
	rep.Dropped++
	slog.WarnContext(ctx, "import: resource skipped, environment_id not found",
		"import.resource.kind", kind, "import.resource.name", name, "import.resource.environment_id", envID)
}

// matchesEnv reports whether env passes the filter. An empty filter matches everything.
func matchesEnv(allow []string, env string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, e := range allow {
		if e == env {
			return true
		}
	}
	return false
}

// dedupeReport sorts and de-duplicates the env-var key lists so the report shows each key
// once in a stable order.
func dedupeReport(rep *Report) {
	rep.EnvKeys = uniqueSorted(rep.EnvKeys)
	rep.PasswordEnvs = uniqueSorted(rep.PasswordEnvs)
	sort.Strings(rep.ServicesSkipped)
	sort.Strings(rep.Databases)
}

func uniqueSorted(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
