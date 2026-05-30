// Package tui implements the read-only `explore` terminal browser for live Coolify state.
package tui

import (
	"context"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

// explorerClient is the read-only subset of *coolify.Client the browser needs (accept
// interfaces, return structs). It is a superset of the resolver's client, so a value of
// this type also satisfies state.Resolve's parameter and can be passed to it directly.
//
// The browser never calls a mutating method: there is no Create/Update/Delete or
// Start/Stop/Restart here by design.
type explorerClient interface {
	ListProjects(ctx context.Context) ([]coolify.Project, error)
	ListEnvironments(ctx context.Context, projectUUID string) ([]coolify.Environment, error)
	ListServers(ctx context.Context) ([]coolify.Server, error)
	ListApplications(ctx context.Context) ([]coolify.Application, error)
	ListServices(ctx context.Context) ([]coolify.Service, error)
	GetServerResources(ctx context.Context, serverUUID string) ([]coolify.ServerResource, error)
	GetApplication(ctx context.Context, uuid string) (coolify.Application, error)
	GetDatabase(ctx context.Context, uuid string) (coolify.Database, error)
	ListServiceEnvs(ctx context.Context, serviceUUID string) ([]coolify.ServiceEnvVar, error)
}
