package tui

import (
	"context"
	"fmt"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

// fakeRecorder captures audit entries instead of writing a file, so lifecycle tracing is
// asserted without touching disk.
type fakeRecorder struct {
	entries []apply.AuditEntry
	err     error
}

func (r *fakeRecorder) Record(e apply.AuditEntry) error {
	r.entries = append(r.entries, e)
	return r.err
}

// fakeClient is an in-memory explorerClient for the browser tests: no HTTP, no httptest,
// fully deterministic. A method whose corresponding *Err field is set returns that error,
// so the error-panel paths are exercised without a network.
type fakeClient struct {
	projects []coolify.Project
	envs     map[string][]coolify.Environment
	servers  []coolify.Server
	apps     []coolify.Application
	services []coolify.Service
	srvRes   map[string][]coolify.ServerResource
	app      coolify.Application
	db       coolify.Database
	svcEnvs  []coolify.ServiceEnvVar

	listProjectsErr error
	getAppErr       error

	// lifecycle records every mutating call so tests assert what was (or was not) invoked.
	lifecycle    []string
	lifecycleErr error
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		projects: []coolify.Project{{ID: 1, UUID: "p1", Name: "restaurant-core"}},
		envs: map[string][]coolify.Environment{
			"p1": {{ID: 10, Name: "staging"}},
		},
		servers:  []coolify.Server{{UUID: "s1", Name: "hetzner"}},
		apps:     []coolify.Application{{UUID: "a1", Name: "web", EnvironmentID: 10}},
		services: []coolify.Service{{UUID: "sv1", Name: "kafka", EnvironmentID: 10}},
		srvRes: map[string][]coolify.ServerResource{
			"s1": {{UUID: "db1", Name: "redis", Type: "standalone-redis", Status: "running"}},
		},
		app: coolify.Application{UUID: "a1", Name: "web", Status: "running:healthy", BuildPack: "nixpacks", FQDN: "https://web.test"},
		db:  coolify.Database{UUID: "db1", Name: "redis", Status: "running", DatabaseType: "standalone-redis"},
		svcEnvs: []coolify.ServiceEnvVar{
			{Key: "KAFKA_PORT", Value: "9092"},
			{Key: "SASL_PASSWORD", Value: "s3cr3t-staging-pwd"},
		},
	}
}

func (f *fakeClient) ListProjects(context.Context) ([]coolify.Project, error) {
	if f.listProjectsErr != nil {
		return nil, f.listProjectsErr
	}
	return f.projects, nil
}

func (f *fakeClient) ListEnvironments(_ context.Context, projectUUID string) ([]coolify.Environment, error) {
	return f.envs[projectUUID], nil
}

func (f *fakeClient) ListServers(context.Context) ([]coolify.Server, error) { return f.servers, nil }

func (f *fakeClient) ListApplications(context.Context) ([]coolify.Application, error) {
	return f.apps, nil
}

func (f *fakeClient) ListServices(context.Context) ([]coolify.Service, error) {
	return f.services, nil
}

func (f *fakeClient) GetServerResources(_ context.Context, serverUUID string) ([]coolify.ServerResource, error) {
	return f.srvRes[serverUUID], nil
}

func (f *fakeClient) GetApplication(_ context.Context, uuid string) (coolify.Application, error) {
	if f.getAppErr != nil {
		return coolify.Application{}, f.getAppErr
	}
	if uuid != f.app.UUID {
		return coolify.Application{}, fmt.Errorf("no application %q", uuid)
	}
	return f.app, nil
}

func (f *fakeClient) StartApplication(_ context.Context, uuid string) error {
	return f.recordLifecycle("start", uuid)
}

func (f *fakeClient) StopApplication(_ context.Context, uuid string) error {
	return f.recordLifecycle("stop", uuid)
}

func (f *fakeClient) RestartApplication(_ context.Context, uuid string) error {
	return f.recordLifecycle("restart", uuid)
}

func (f *fakeClient) recordLifecycle(action, uuid string) error {
	f.lifecycle = append(f.lifecycle, action+":"+uuid)
	return f.lifecycleErr
}

func (f *fakeClient) GetDatabase(_ context.Context, uuid string) (coolify.Database, error) {
	if uuid != f.db.UUID {
		return coolify.Database{}, fmt.Errorf("no database %q", uuid)
	}
	return f.db, nil
}

func (f *fakeClient) ListServiceEnvs(context.Context, string) ([]coolify.ServiceEnvVar, error) {
	return f.svcEnvs, nil
}
