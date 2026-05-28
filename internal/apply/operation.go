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
// (e.g. "${env:DATABASE_URL}"), never their values. Only an Application carries secrets,
// via its inline env vars.
func secretSources(op Operation) []string {
	if op.ApplicationSpec == nil {
		return nil
	}
	var out []string
	for _, ev := range op.ApplicationSpec.Spec.EnvVars {
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

func serverKey(name string) state.ResourceKey {
	return state.ResourceKey{Kind: state.KindServer, Name: name}
}
