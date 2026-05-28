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
		PortsExposes:    strconv.Itoa(app.Spec.Port),
	}
	if app.Spec.Image != nil {
		req.DockerRegistryImageName = app.Spec.Image.Name
		req.DockerRegistryImageTag = app.Spec.Image.Tag
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

func auditEntryFor(op Operation) AuditEntry {
	return AuditEntry{
		Operation: "apply",
		Resource:  resourceLabel(op),
		Op:        string(op.Op),
		Sources:   secretSources(op),
		DiffHash:  diffHash(op),
	}
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
