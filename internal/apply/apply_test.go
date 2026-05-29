package apply_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// the real client must satisfy the reconciler's interface.
var _ apply.Client = (*coolify.Client)(nil)

type recordedCall struct {
	method      string
	envProjUUID string
	appReq      coolify.CreateApplicationRequest
	updReq      coolify.UpdateApplicationRequest
	svcReq      coolify.CreateServiceRequest
	envs        []coolify.ServiceEnvVar
	uuid        string
}

type mockClient struct {
	calls  []recordedCall
	failOn string // method name to fail on
}

func (m *mockClient) fail(method string) error {
	if m.failOn == method {
		return fmt.Errorf("boom in %s", method)
	}
	return nil
}

func (m *mockClient) CreateProject(_ context.Context, _ coolify.CreateProjectRequest) (string, error) {
	m.calls = append(m.calls, recordedCall{method: "CreateProject"})
	return "proj-uuid", m.fail("CreateProject")
}

func (m *mockClient) DeleteProject(_ context.Context, uuid string) error {
	m.calls = append(m.calls, recordedCall{method: "DeleteProject", uuid: uuid})
	return m.fail("DeleteProject")
}

func (m *mockClient) CreateEnvironment(_ context.Context, projectUUID string, _ coolify.CreateEnvironmentRequest) (string, error) {
	m.calls = append(m.calls, recordedCall{method: "CreateEnvironment", envProjUUID: projectUUID})
	return "env-uuid", m.fail("CreateEnvironment")
}

func (m *mockClient) DeleteEnvironment(_ context.Context, projectUUID, _ string) error {
	m.calls = append(m.calls, recordedCall{method: "DeleteEnvironment", envProjUUID: projectUUID})
	return m.fail("DeleteEnvironment")
}

func (m *mockClient) CreateApplication(_ context.Context, req coolify.CreateApplicationRequest) (string, error) {
	m.calls = append(m.calls, recordedCall{method: "CreateApplication", appReq: req})
	return "app-uuid", m.fail("CreateApplication")
}

func (m *mockClient) UpdateApplication(_ context.Context, uuid string, req coolify.UpdateApplicationRequest) error {
	m.calls = append(m.calls, recordedCall{method: "UpdateApplication", uuid: uuid, updReq: req})
	return m.fail("UpdateApplication")
}

func (m *mockClient) DeleteApplication(_ context.Context, uuid string) error {
	m.calls = append(m.calls, recordedCall{method: "DeleteApplication", uuid: uuid})
	return m.fail("DeleteApplication")
}

func (m *mockClient) CreateService(_ context.Context, req coolify.CreateServiceRequest) (string, error) {
	m.calls = append(m.calls, recordedCall{method: "CreateService", svcReq: req})
	return "svc-uuid", m.fail("CreateService")
}

func (m *mockClient) UpdateService(_ context.Context, uuid string, _ coolify.UpdateServiceRequest) error {
	m.calls = append(m.calls, recordedCall{method: "UpdateService", uuid: uuid})
	return m.fail("UpdateService")
}

func (m *mockClient) DeleteService(_ context.Context, uuid string) error {
	m.calls = append(m.calls, recordedCall{method: "DeleteService", uuid: uuid})
	return m.fail("DeleteService")
}

func (m *mockClient) BulkUpdateServiceEnvs(_ context.Context, serviceUUID string, envs []coolify.ServiceEnvVar) error {
	m.calls = append(m.calls, recordedCall{method: "BulkUpdateServiceEnvs", uuid: serviceUUID, envs: envs})
	return m.fail("BulkUpdateServiceEnvs")
}

func dockerimageApp() resource.Application {
	return resource.Application{
		Metadata: resource.ApplicationMeta{Name: "api", Project: "beenaire", Environment: "staging"},
		Spec: resource.ApplicationSpec{
			BuildPack:   "dockerimage",
			Image:       &resource.ImageSpec{Name: "registry/api", Tag: "v1"},
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			FQDN:        "https://api.example.com",
			Port:        8000,
		},
	}
}

func serverResolved() state.Map {
	return state.Map{state.ResourceKey{Kind: state.KindServer, Name: "localhost"}: "srv-uuid"}
}

func TestApplyCreateThreadsParentUUIDs(t *testing.T) {
	mc := &mockClient{}
	ops, err := apply.OrderApply([]apply.Operation{
		apply.CreateProjectOp(resource.Project{Metadata: resource.ProjectMeta{Name: "beenaire"}}),
		apply.CreateEnvironmentOp(resource.Environment{Metadata: resource.EnvironmentMeta{Name: "staging", Project: "beenaire"}}),
		apply.ApplicationOp(apply.OpCreate, dockerimageApp(), nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	eng := apply.NewEngine(mc, serverResolved(), nil)
	sum, err := eng.Apply(context.Background(), ops)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if sum.Applied != 3 || sum.Failed != 0 {
		t.Fatalf("summary = %+v, want 3 applied", sum)
	}
	if len(mc.calls) != 3 {
		t.Fatalf("want 3 client calls, got %d", len(mc.calls))
	}
	if mc.calls[0].method != "CreateProject" || mc.calls[1].method != "CreateEnvironment" || mc.calls[2].method != "CreateApplication" {
		t.Fatalf("call order = %v", methods(mc.calls))
	}
	// Environment create must receive the project UUID minted moments earlier.
	if mc.calls[1].envProjUUID != "proj-uuid" {
		t.Errorf("CreateEnvironment projectUUID = %q, want proj-uuid (threaded)", mc.calls[1].envProjUUID)
	}
	app := mc.calls[2].appReq
	if app.ProjectUUID != "proj-uuid" || app.ServerUUID != "srv-uuid" || app.EnvironmentName != "staging" {
		t.Errorf("create application request not wired from resolved state: %+v", app)
	}
	if app.DockerRegistryImageName != "registry/api" || app.PortsExposes != "8000" {
		t.Errorf("create application request missing image/port: %+v", app)
	}
}

func TestApplyPartialFailureReportsApplied(t *testing.T) {
	mc := &mockClient{failOn: "CreateEnvironment"}
	ops, err := apply.OrderApply([]apply.Operation{
		apply.CreateProjectOp(resource.Project{Metadata: resource.ProjectMeta{Name: "beenaire"}}),
		apply.CreateEnvironmentOp(resource.Environment{Metadata: resource.EnvironmentMeta{Name: "staging", Project: "beenaire"}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	sum, err := apply.NewEngine(mc, serverResolved(), nil).Apply(context.Background(), ops)
	if err == nil {
		t.Fatal("want error when an operation fails")
	}
	if sum.Applied != 1 || sum.Failed != 1 {
		t.Errorf("summary = %+v, want 1 applied 1 failed (partial)", sum)
	}
}

func TestApplyFirstOpFailureIsNotPartial(t *testing.T) {
	mc := &mockClient{failOn: "CreateProject"}
	ops := []apply.Operation{apply.CreateProjectOp(resource.Project{Metadata: resource.ProjectMeta{Name: "beenaire"}})}
	sum, err := apply.NewEngine(mc, nil, nil).Apply(context.Background(), ops)
	if err == nil {
		t.Fatal("want error")
	}
	if sum.Applied != 0 {
		t.Errorf("Applied = %d, want 0 (first op failed)", sum.Applied)
	}
}

func TestApplyUpdateMapsChangedFields(t *testing.T) {
	mc := &mockClient{}
	resolved := serverResolved()
	resolved[state.ResourceKey{Project: "beenaire", Environment: "staging", Kind: resource.KindApplication, Name: "api"}] = "app-uuid"
	changes := []plan.Change{
		{Op: plan.OpUpdate, Path: "Application.api.fqdn", New: "https://new.example.com"},
		{Op: plan.OpUpdate, Path: "Application.api.image.tag", New: "v2"},
	}
	ops := []apply.Operation{apply.ApplicationOp(apply.OpUpdate, dockerimageApp(), changes)}
	if _, err := apply.NewEngine(mc, resolved, nil).Apply(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	if mc.calls[0].method != "UpdateApplication" || mc.calls[0].uuid != "app-uuid" {
		t.Fatalf("want UpdateApplication app-uuid, got %+v", mc.calls[0])
	}
	upd := mc.calls[0].updReq
	if upd.Domains != "https://new.example.com" || upd.DockerRegistryImageTag != "v2" {
		t.Errorf("patch body = %+v, want fqdn+tag mapped", upd)
	}
	if upd.PortsExposes != "" {
		t.Errorf("unchanged port must be omitted from patch, got %q", upd.PortsExposes)
	}
}

func TestApplyCreateUnresolvedServerErrors(t *testing.T) {
	mc := &mockClient{}
	// resolved map has the project but not the server.
	resolved := state.Map{state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}: "proj-uuid"}
	ops := []apply.Operation{apply.ApplicationOp(apply.OpCreate, dockerimageApp(), nil)}
	if _, err := apply.NewEngine(mc, resolved, nil).Apply(context.Background(), ops); err == nil {
		t.Fatal("want error when the destination server is unresolved")
	}
	if len(mc.calls) != 0 {
		t.Errorf("no create call should be made when a parent is unresolved")
	}
}

func methods(calls []recordedCall) []string {
	out := make([]string, len(calls))
	for i, c := range calls {
		out[i] = c.method
	}
	return out
}
