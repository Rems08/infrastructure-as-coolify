// Package apply reconciles desired iac-coolify resources with a live Coolify instance. It
// orders operations by dependency (project → environment → application), applies them
// sequentially, and appends an audit record per applied operation. Secret values never
// reach the audit log: only their source declarations do.
package apply

import (
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// Op is the mutation a single operation performs on its target resource.
type Op string

const (
	// OpCreate creates a resource that does not exist remotely.
	OpCreate Op = "create"
	// OpUpdate patches a resource whose desired fields differ from the remote state.
	OpUpdate Op = "update"
	// OpDelete removes a resource.
	OpDelete Op = "delete"
)

// Operation is one resource-level mutation in an apply run. Kind selects which of the
// resource pointers is meaningful. Project/Environment/Name are the logical coordinates
// used for dependency ordering, parent-UUID lookup and audit labelling.
type Operation struct {
	Op          Op
	Kind        string
	Project     string
	Environment string
	Name        string

	ProjectSpec     *resource.Project
	EnvironmentSpec *resource.Environment
	ApplicationSpec *resource.Application
	ServiceSpec     *resource.Service

	// ServiceComposeRaw is the decoded docker-compose content for a compose_path Service,
	// read and path-checked at load time. It is empty for a one-click (type) Service and
	// for every non-Service operation. The engine base64-encodes it via the client and
	// records only its hash in the audit log, never the content.
	ServiceComposeRaw string

	// Changes carries the field-level diff for an update (used to build the patch body
	// and the audit diff hash). It is empty for create and delete.
	Changes []plan.Change
}

// CreateProjectOp builds a create operation for a Project.
func CreateProjectOp(p resource.Project) Operation {
	return Operation{Op: OpCreate, Kind: resource.KindProject, Name: p.Metadata.Name, ProjectSpec: &p}
}

// CreateEnvironmentOp builds a create operation for an Environment.
func CreateEnvironmentOp(e resource.Environment) Operation {
	return Operation{
		Op:              OpCreate,
		Kind:            resource.KindEnvironment,
		Project:         e.Metadata.Project,
		Name:            e.Metadata.Name,
		EnvironmentSpec: &e,
	}
}

// ApplicationOp builds an operation (create/update/delete) for an Application. For an
// update, changes carries the field-level diff that drives the patch.
func ApplicationOp(op Op, app resource.Application, changes []plan.Change) Operation {
	return Operation{
		Op:              op,
		Kind:            resource.KindApplication,
		Project:         app.Metadata.Project,
		Environment:     app.Metadata.Environment,
		Name:            app.Metadata.Name,
		ApplicationSpec: &app,
		Changes:         changes,
	}
}

// ServiceOp builds an operation (create/update/delete) for a Service. composeRaw is the
// decoded compose content for a compose_path service ("" for a one-click template or a
// delete).
func ServiceOp(op Op, svc resource.Service, composeRaw string, changes []plan.Change) Operation {
	return Operation{
		Op:                op,
		Kind:              resource.KindService,
		Project:           svc.Metadata.Project,
		Environment:       svc.Metadata.Environment,
		Name:              svc.Metadata.Name,
		ServiceSpec:       &svc,
		ServiceComposeRaw: composeRaw,
		Changes:           changes,
	}
}

// resourceLabel renders an operation's target as "Kind/project/environment/name",
// skipping empty coordinates. Used in audit records and error messages.
func resourceLabel(op Operation) string {
	segs := []string{op.Kind}
	for _, s := range []string{op.Project, op.Environment, op.Name} {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return strings.Join(segs, "/")
}

// secretSources returns the source declarations of every Secret an operation references
// (e.g. "${env:DATABASE_URL}"), never their values. Applications and Services carry
// secrets via their inline env vars.
func secretSources(op Operation) []string {
	var entries []resource.EnvVarEntry
	switch {
	case op.ApplicationSpec != nil:
		entries = op.ApplicationSpec.Spec.EnvVars
	case op.ServiceSpec != nil:
		entries = op.ServiceSpec.Spec.EnvVars
	}
	var out []string
	for _, ev := range entries {
		if !ev.ValueSecret.IsZero() {
			out = append(out, ev.ValueSecret.Origin())
		}
	}
	return out
}

func projectKey(name string) state.ResourceKey {
	return state.ResourceKey{Kind: resource.KindProject, Name: name}
}

func envKey(project, name string) state.ResourceKey {
	return state.ResourceKey{Project: project, Kind: resource.KindEnvironment, Name: name}
}

func appKey(project, environment, name string) state.ResourceKey {
	return state.ResourceKey{Project: project, Environment: environment, Kind: resource.KindApplication, Name: name}
}

func serviceKey(project, environment, name string) state.ResourceKey {
	return state.ResourceKey{Project: project, Environment: environment, Kind: resource.KindService, Name: name}
}

func serverKey(name string) state.ResourceKey {
	return state.ResourceKey{Kind: state.KindServer, Name: name}
}
