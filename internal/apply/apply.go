package apply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// Client is the subset of the Coolify client the reconciler needs (accept interfaces).
// *coolify.Client satisfies it.
type Client interface {
	CreateProject(ctx context.Context, req coolify.CreateProjectRequest) (string, error)
	DeleteProject(ctx context.Context, uuid string) error
	CreateEnvironment(ctx context.Context, projectUUID string, req coolify.CreateEnvironmentRequest) (string, error)
	DeleteEnvironment(ctx context.Context, projectUUID, envNameOrUUID string) error
	CreateApplication(ctx context.Context, req coolify.CreateApplicationRequest) (string, error)
	UpdateApplication(ctx context.Context, uuid string, req coolify.UpdateApplicationRequest) error
	DeleteApplication(ctx context.Context, uuid string) error
	CreateService(ctx context.Context, req coolify.CreateServiceRequest) (string, error)
	UpdateService(ctx context.Context, uuid string, req coolify.UpdateServiceRequest) error
	DeleteService(ctx context.Context, uuid string) error
	BulkUpdateServiceEnvs(ctx context.Context, serviceUUID string, envs []coolify.ServiceEnvVar) error
	CreateDatabase(ctx context.Context, req coolify.DatabaseCreateRequest) (string, error)
	UpdateDatabase(ctx context.Context, uuid string, req coolify.UpdateDatabaseRequest) error
	DeleteDatabase(ctx context.Context, uuid string, opts coolify.DeleteDatabaseOptions) error
}

// Summary counts the outcome of an apply run.
type Summary struct {
	Applied int
	Failed  int
	Errors  []error
}

// Engine reconciles operations against a Coolify instance. resolved holds the live
// logical-key → identifier map; it is updated in place as creates yield new UUIDs, so
// later operations can resolve their freshly-created parents.
type Engine struct {
	client   Client
	resolved state.Map
	auditor  *Auditor // nil disables audit logging
}

// NewEngine returns an Engine. A nil resolved map is treated as empty; a nil auditor
// disables audit logging.
func NewEngine(client Client, resolved state.Map, auditor *Auditor) *Engine {
	if resolved == nil {
		resolved = state.Map{}
	}
	return &Engine{client: client, resolved: resolved, auditor: auditor}
}

// Apply executes ops in the given order, stopping at the first failure (no partial
// rollback — the audit log lets an operator reconcile or revert manually). On a failure
// after at least one success, Summary.Applied is > 0 so the caller can exit with the
// partial-success code.
func (e *Engine) Apply(ctx context.Context, ops []Operation) (Summary, error) {
	var s Summary
	for _, op := range ops {
		if err := e.applyOne(ctx, op); err != nil {
			s.Failed++
			s.Errors = append(s.Errors, err)
			return s, fmt.Errorf("apply %s %s: %w", op.Op, resourceLabel(op), err)
		}
		if e.auditor != nil {
			if aErr := e.auditor.Record(auditEntryFor(op)); aErr != nil {
				return s, fmt.Errorf("apply: %w", aErr)
			}
		}
		s.Applied++
	}
	return s, nil
}

func (e *Engine) applyOne(ctx context.Context, op Operation) error {
	switch op.Kind {
	case resource.KindProject:
		return e.applyProject(ctx, op)
	case resource.KindEnvironment:
		return e.applyEnvironment(ctx, op)
	case resource.KindApplication:
		return e.applyApplication(ctx, op)
	case resource.KindService:
		return e.applyService(ctx, op)
	case resource.KindDatabase:
		return e.applyDatabase(ctx, op)
	default:
		return fmt.Errorf("unsupported kind %q", op.Kind)
	}
}

func (e *Engine) applyProject(ctx context.Context, op Operation) error {
	switch op.Op {
	case OpCreate:
		uuid, err := e.client.CreateProject(ctx, coolify.CreateProjectRequest{
			Name:        op.Name,
			Description: op.ProjectSpec.Spec.Description,
		})
		if err != nil {
			return err
		}
		e.resolved[projectKey(op.Name)] = uuid
		return nil
	case OpDelete:
		uuid, ok := e.resolved.Lookup(projectKey(op.Name))
		if !ok {
			return nil // already absent
		}
		return e.client.DeleteProject(ctx, uuid)
	default:
		return fmt.Errorf("unsupported op %q for project", op.Op)
	}
}

func (e *Engine) applyEnvironment(ctx context.Context, op Operation) error {
	projUUID, ok := e.resolved.Lookup(projectKey(op.Project))
	switch op.Op {
	case OpCreate:
		if !ok {
			return fmt.Errorf("project %q not resolved", op.Project)
		}
		if _, err := e.client.CreateEnvironment(ctx, projUUID, coolify.CreateEnvironmentRequest{Name: op.Name}); err != nil {
			return err
		}
		e.resolved[envKey(op.Project, op.Name)] = op.Name
		return nil
	case OpDelete:
		if !ok {
			return nil // parent project gone; nothing to delete
		}
		return e.client.DeleteEnvironment(ctx, projUUID, op.Name)
	default:
		return fmt.Errorf("unsupported op %q for environment", op.Op)
	}
}

func (e *Engine) applyApplication(ctx context.Context, op Operation) error {
	switch op.Op {
	case OpCreate:
		req, err := e.applicationCreateRequest(op)
		if err != nil {
			return err
		}
		uuid, err := e.client.CreateApplication(ctx, req)
		if err != nil {
			return err
		}
		e.resolved[appKey(op.Project, op.Environment, op.Name)] = uuid
		return nil
	case OpUpdate:
		uuid, ok := e.resolved.Lookup(appKey(op.Project, op.Environment, op.Name))
		if !ok {
			return fmt.Errorf("application not resolved")
		}
		return e.client.UpdateApplication(ctx, uuid, updateRequestFromChanges(op))
	case OpDelete:
		uuid, ok := e.resolved.Lookup(appKey(op.Project, op.Environment, op.Name))
		if !ok {
			return nil
		}
		return e.client.DeleteApplication(ctx, uuid)
	default:
		return fmt.Errorf("unsupported op %q for application", op.Op)
	}
}

func (e *Engine) applyService(ctx context.Context, op Operation) error {
	switch op.Op {
	case OpCreate:
		req, err := e.serviceCreateRequest(op)
		if err != nil {
			return err
		}
		uuid, err := e.client.CreateService(ctx, req)
		if err != nil {
			return err
		}
		e.resolved[serviceKey(op.Project, op.Environment, op.Name)] = uuid
		return e.applyServiceEnvs(ctx, uuid, op)
	case OpUpdate:
		uuid, ok := e.resolved.Lookup(serviceKey(op.Project, op.Environment, op.Name))
		if !ok {
			return fmt.Errorf("service not resolved")
		}
		if err := e.client.UpdateService(ctx, uuid, coolify.UpdateServiceRequest{
			Name:             op.Name,
			Description:      op.ServiceSpec.Spec.Description,
			DockerComposeRaw: op.ServiceComposeRaw,
		}); err != nil {
			return err
		}
		return e.applyServiceEnvs(ctx, uuid, op)
	case OpDelete:
		uuid, ok := e.resolved.Lookup(serviceKey(op.Project, op.Environment, op.Name))
		if !ok {
			return nil
		}
		return e.client.DeleteService(ctx, uuid)
	default:
		return fmt.Errorf("unsupported op %q for service", op.Op)
	}
}

func (e *Engine) applyServiceEnvs(ctx context.Context, uuid string, op Operation) error {
	envs := serviceEnvVars(op.ServiceSpec.Spec.EnvVars)
	if len(envs) == 0 {
		return nil
	}
	return e.client.BulkUpdateServiceEnvs(ctx, uuid, envs)
}

func (e *Engine) applyDatabase(ctx context.Context, op Operation) error {
	switch op.Op {
	case OpCreate:
		req, err := e.databaseCreateRequest(op)
		if err != nil {
			return err
		}
		uuid, err := e.client.CreateDatabase(ctx, req)
		if err != nil {
			return err
		}
		e.resolved[databaseKey(op.Name)] = uuid
		return nil
	case OpUpdate:
		uuid, ok := e.resolved.Lookup(databaseKey(op.Name))
		if !ok {
			return fmt.Errorf("database not resolved")
		}
		return e.client.UpdateDatabase(ctx, uuid, updateDatabaseRequestFromChanges(op))
	case OpDelete:
		uuid, ok := e.resolved.Lookup(databaseKey(op.Name))
		if !ok {
			return nil
		}
		return e.client.DeleteDatabase(ctx, uuid, coolify.DefaultDeleteDatabaseOptions())
	default:
		return fmt.Errorf("unsupported op %q for database", op.Op)
	}
}

// serviceCreateRequest builds the create body from the desired spec and the resolved
// parent UUIDs (project and destination server). EnvironmentUUID is left empty: v4
// environments have no UUID and are addressed by name, so only environment_name is sent.
func (e *Engine) serviceCreateRequest(op Operation) (coolify.CreateServiceRequest, error) {
	svc := op.ServiceSpec
	projUUID, ok := e.resolved.Lookup(projectKey(svc.Metadata.Project))
	if !ok {
		return coolify.CreateServiceRequest{}, fmt.Errorf("project %q not resolved", svc.Metadata.Project)
	}
	srvUUID, ok := e.resolved.Lookup(serverKey(svc.Spec.Destination.Server))
	if !ok {
		return coolify.CreateServiceRequest{}, fmt.Errorf("server %q not resolved", svc.Spec.Destination.Server)
	}
	return coolify.CreateServiceRequest{
		Type:             svc.Spec.Type,
		Name:             svc.Metadata.Name,
		Description:      svc.Spec.Description,
		ProjectUUID:      projUUID,
		EnvironmentName:  svc.Metadata.Environment,
		ServerUUID:       srvUUID,
		InstantDeploy:    svc.Spec.InstantDeploy,
		DockerComposeRaw: op.ServiceComposeRaw,
	}, nil
}

// serviceEnvVars maps a service's declared env vars to client env vars, carrying each
// Secret intact (revealed only later, at the HTTP boundary) so no value is exposed here.
func serviceEnvVars(entries []resource.EnvVarEntry) []coolify.ServiceEnvVar {
	out := make([]coolify.ServiceEnvVar, 0, len(entries))
	for _, e := range entries {
		ev := coolify.ServiceEnvVar{Key: e.Name, Value: e.Value}
		if !e.ValueSecret.IsZero() {
			ev.Secret = e.ValueSecret
		}
		out = append(out, ev)
	}
	return out
}

// applicationCreateRequest builds the create body from the desired spec and the resolved
// parent UUIDs (project and destination server).
func (e *Engine) applicationCreateRequest(op Operation) (coolify.CreateApplicationRequest, error) {
	app := op.ApplicationSpec
	projUUID, ok := e.resolved.Lookup(projectKey(app.Metadata.Project))
	if !ok {
		return coolify.CreateApplicationRequest{}, fmt.Errorf("project %q not resolved", app.Metadata.Project)
	}
	srvUUID, ok := e.resolved.Lookup(serverKey(app.Spec.Destination.Server))
	if !ok {
		return coolify.CreateApplicationRequest{}, fmt.Errorf("server %q not resolved", app.Spec.Destination.Server)
	}
	req := coolify.CreateApplicationRequest{
		BuildPack:       app.Spec.BuildPack,
		ProjectUUID:     projUUID,
		ServerUUID:      srvUUID,
		EnvironmentName: app.Metadata.Environment,
		Name:            app.Metadata.Name,
		Domains:         app.Spec.FQDN,
		Dockerfile:      app.Spec.Dockerfile,
	}
	switch {
	case app.Spec.Image != nil:
		req.DockerRegistryImageName = app.Spec.Image.Name
		req.DockerRegistryImageTag = app.Spec.Image.Tag
	case app.Spec.Source != nil:
		req.GitRepository = app.Spec.Source.GitRepository
		req.GitBranch = app.Spec.Source.GitBranch
	}
	// A git source carries its own ports_exposes; otherwise the optional top-level port
	// (required for dockerimage, optional for an inline Dockerfile) supplies it.
	if app.Spec.Source != nil {
		req.PortsExposes = app.Spec.Source.PortsExposes
	} else if app.Spec.Port > 0 {
		req.PortsExposes = strconv.Itoa(app.Spec.Port)
	}
	return req, nil
}

// updateRequestFromChanges maps an application's changed diff fields to the PATCH body.
func updateRequestFromChanges(op Operation) coolify.UpdateApplicationRequest {
	prefix := op.Kind + "." + op.Name + "."
	var req coolify.UpdateApplicationRequest
	for _, c := range op.Changes {
		switch strings.TrimPrefix(c.Path, prefix) {
		case "fqdn":
			req.Domains = c.New
		case "port":
			req.PortsExposes = c.New
		case "image.name":
			req.DockerRegistryImageName = c.New
		case "image.tag":
			req.DockerRegistryImageTag = c.New
		}
	}
	return req
}

// databaseCommon resolves the parent UUIDs and builds the create-body fields shared by
// every engine.
func (e *Engine) databaseCommon(db resource.Database) (coolify.CreateDatabaseCommon, error) {
	projUUID, ok := e.resolved.Lookup(projectKey(db.Metadata.Project))
	if !ok {
		return coolify.CreateDatabaseCommon{}, fmt.Errorf("project %q not resolved", db.Metadata.Project)
	}
	srvUUID, ok := e.resolved.Lookup(serverKey(db.Spec.Destination.Server))
	if !ok {
		return coolify.CreateDatabaseCommon{}, fmt.Errorf("server %q not resolved", db.Spec.Destination.Server)
	}
	common := coolify.CreateDatabaseCommon{
		ServerUUID:      srvUUID,
		ProjectUUID:     projUUID,
		EnvironmentName: db.Metadata.Environment,
		Name:            db.Metadata.Name,
		Image:           db.Spec.Image,
		IsPublic:        db.Spec.Public,
		PublicPort:      db.Spec.PublicPort,
	}
	if db.Spec.Limits != nil {
		common.LimitsCPUShares = db.Spec.Limits.CPUShares
		common.LimitsMemory = db.Spec.Limits.Memory
	}
	return common, nil
}

// databaseCreateRequest builds the engine-specific create request, mapping the single
// declared password to the engine's primary credential field. MongoDB exposes no
// create-time password field in the pinned v4 spec, so its password is not sent on create.
func (e *Engine) databaseCreateRequest(op Operation) (coolify.DatabaseCreateRequest, error) {
	db := *op.DatabaseSpec
	common, err := e.databaseCommon(db)
	if err != nil {
		return nil, err
	}
	pw := db.Spec.Password
	switch db.Spec.Engine {
	case "postgresql":
		return coolify.CreateDatabasePostgresqlRequest{CreateDatabaseCommon: common, PostgresPassword: pw}, nil
	case "mysql":
		return coolify.CreateDatabaseMysqlRequest{CreateDatabaseCommon: common, MySQLPassword: pw}, nil
	case "mariadb":
		return coolify.CreateDatabaseMariadbRequest{CreateDatabaseCommon: common, MariaDBPassword: pw}, nil
	case "mongodb":
		return coolify.CreateDatabaseMongodbRequest{CreateDatabaseCommon: common}, nil
	case "redis":
		return coolify.CreateDatabaseRedisRequest{CreateDatabaseCommon: common, RedisPassword: pw}, nil
	case "keydb":
		return coolify.CreateDatabaseKeydbRequest{CreateDatabaseCommon: common, KeyDBPassword: pw}, nil
	case "dragonfly":
		return coolify.CreateDatabaseDragonflyRequest{CreateDatabaseCommon: common, DragonflyPassword: pw}, nil
	case "clickhouse":
		return coolify.CreateDatabaseClickhouseRequest{CreateDatabaseCommon: common, ClickhouseAdminPassword: pw}, nil
	default:
		return nil, fmt.Errorf("unsupported database engine %q", db.Spec.Engine)
	}
}

// updateDatabaseRequestFromChanges maps a database's changed diff fields to the PATCH body.
func updateDatabaseRequestFromChanges(op Operation) coolify.UpdateDatabaseRequest {
	prefix := op.Kind + "." + op.Name + "."
	var req coolify.UpdateDatabaseRequest
	for _, c := range op.Changes {
		switch strings.TrimPrefix(c.Path, prefix) {
		case "image":
			req.Image = c.New
		case "public":
			b := c.New == "true"
			req.IsPublic = &b
		case "public_port":
			if n, err := strconv.Atoi(c.New); err == nil {
				req.PublicPort = n
			}
		case "limits.cpu_shares":
			if n, err := strconv.Atoi(c.New); err == nil {
				req.LimitsCPUShares = n
			}
		case "limits.memory":
			req.LimitsMemory = c.New
		}
	}
	return req
}

func auditEntryFor(op Operation) AuditEntry {
	return AuditEntry{
		Operation:   "apply",
		Resource:    resourceLabel(op),
		Op:          string(op.Op),
		Sources:     secretSources(op),
		DiffHash:    diffHash(op),
		ComposeHash: composeHash(op),
	}
}

// composeHash returns sha256 of a Service's decoded compose content, or "" when there is
// none. The content never reaches the audit log; only this hash does.
func composeHash(op Operation) string {
	if op.ServiceComposeRaw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(op.ServiceComposeRaw))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// diffHash is a sha256 over the operation's redaction-safe diff (paths and display-safe
// new values), used for change detection. It never incorporates a secret value: a secret
// change's New is the source declaration or a redacted note, not the value.
func diffHash(op Operation) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n", resourceLabel(op), op.Op)
	for _, c := range op.Changes {
		fmt.Fprintf(h, "%s=%s\n", c.Path, c.New)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}
