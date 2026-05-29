package apply

import (
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// DeleteInput is the desired set of resources to destroy plus the live state used to decide
// which actually exist remotely.
type DeleteInput struct {
	Projects     []resource.Project
	Environments []resource.Environment
	Applications []resource.Application
	Services     []resource.Service
	Databases    []resource.Database
	// Resolved maps logical keys to live identifiers. A resource absent from it does not
	// exist remotely and is skipped — unless AssumePresent is set.
	Resolved state.Map
	// Only, when non-empty, restricts the plan to the resource with this logical name.
	Only string
	// AssumePresent includes every declared resource even when it is not in Resolved. It is
	// for an offline --dry-run preview, where the live state is unknown.
	AssumePresent bool
}

// DeleteOperations builds the delete operations for the declared resources that exist
// remotely (or all of them when AssumePresent is set), filtered by Only. The result is
// unordered; callers pass it through OrderDelete to get the safe reverse-dependency order
// (applications and services before environments before projects).
func (in DeleteInput) DeleteOperations() []Operation {
	var ops []Operation
	for _, app := range in.Applications {
		if in.includes(in.Only, app.Metadata.Name, appKey(app.Metadata.Project, app.Metadata.Environment, app.Metadata.Name)) {
			ops = append(ops, ApplicationOp(OpDelete, app, nil))
		}
	}
	for _, svc := range in.Services {
		if in.includes(in.Only, svc.Metadata.Name, serviceKey(svc.Metadata.Project, svc.Metadata.Environment, svc.Metadata.Name)) {
			ops = append(ops, ServiceOp(OpDelete, svc, "", nil))
		}
	}
	for _, db := range in.Databases {
		if in.includes(in.Only, db.Metadata.Name, databaseKey(db.Metadata.Name)) {
			ops = append(ops, DatabaseOp(OpDelete, db, nil))
		}
	}
	for _, e := range in.Environments {
		if in.includes(in.Only, e.Metadata.Name, envKey(e.Metadata.Project, e.Metadata.Name)) {
			ops = append(ops, DeleteEnvironmentOp(e))
		}
	}
	for _, p := range in.Projects {
		if in.includes(in.Only, p.Metadata.Name, projectKey(p.Metadata.Name)) {
			ops = append(ops, DeleteProjectOp(p))
		}
	}
	return ops
}

// includes reports whether a resource passes the name filter and is present remotely (or is
// assumed present for an offline preview).
func (in DeleteInput) includes(only, name string, key state.ResourceKey) bool {
	if only != "" && only != name {
		return false
	}
	return in.AssumePresent || in.Resolved.Has(key)
}
