package apply_test

import (
	"context"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

func database(name, engine string) resource.Database {
	return resource.Database{
		Metadata: resource.DatabaseMeta{Name: name, Project: "beenaire", Environment: "staging"},
		Spec: resource.DatabaseSpec{
			Engine:      engine,
			Image:       engine + ":latest",
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			Password:    secrets.NewRemote("db-password"),
		},
	}
}

// projectResolved adds the project and server UUIDs a database create needs.
func projectResolved() state.Map {
	m := serverResolved()
	m[state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}] = "proj-uuid"
	return m
}

func TestApplyDatabaseCreatePostgres(t *testing.T) {
	mc := &mockClient{}
	ops := []apply.Operation{apply.DatabaseOp(apply.OpCreate, database("pg", "postgresql"), nil)}
	if _, err := apply.NewEngine(mc, projectResolved(), nil).Apply(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	if len(mc.calls) != 1 || mc.calls[0].method != "CreateDatabase" {
		t.Fatalf("want one CreateDatabase call, got %v", methods(mc.calls))
	}
	req, ok := mc.calls[0].dbReq.(coolify.CreateDatabasePostgresqlRequest)
	if !ok {
		t.Fatalf("create request type = %T, want CreateDatabasePostgresqlRequest", mc.calls[0].dbReq)
	}
	if req.ProjectUUID != "proj-uuid" || req.ServerUUID != "srv-uuid" || req.EnvironmentName != "staging" {
		t.Errorf("create request not wired from resolved state: %+v", req.CreateDatabaseCommon)
	}
	if !req.PostgresPassword.ValueEquals(secrets.NewRemote("db-password")) {
		t.Errorf("declared password not carried into the engine credential field")
	}
}

func TestApplyDatabaseCreateUnresolvedParentFails(t *testing.T) {
	mc := &mockClient{}
	// No project in the resolved map: the create must fail before any client call.
	ops := []apply.Operation{apply.DatabaseOp(apply.OpCreate, database("pg", "postgresql"), nil)}
	if _, err := apply.NewEngine(mc, serverResolved(), nil).Apply(context.Background(), ops); err == nil {
		t.Fatal("want error when the parent project is unresolved")
	}
	if len(mc.calls) != 0 {
		t.Errorf("no client call expected on an unresolved parent, got %d", len(mc.calls))
	}
}

func TestApplyDatabaseUpdateMapsChangedFields(t *testing.T) {
	mc := &mockClient{}
	resolved := projectResolved()
	resolved[state.ResourceKey{Kind: resource.KindDatabase, Name: "cache"}] = "db-uuid"
	changes := []plan.Change{
		{Op: plan.OpUpdate, Path: "Database.cache.image", New: "redis:8-alpine"},
		{Op: plan.OpUpdate, Path: "Database.cache.limits.memory", New: "512m"},
	}
	ops := []apply.Operation{apply.DatabaseOp(apply.OpUpdate, database("cache", "redis"), changes)}
	if _, err := apply.NewEngine(mc, resolved, nil).Apply(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	if mc.calls[0].method != "UpdateDatabase" || mc.calls[0].uuid != "db-uuid" {
		t.Fatalf("want UpdateDatabase db-uuid, got %+v", mc.calls[0])
	}
	upd := mc.calls[0].dbUpdReq
	if upd.Image != "redis:8-alpine" || upd.LimitsMemory != "512m" {
		t.Errorf("patch body = %+v, want image+memory mapped", upd)
	}
	// An unchanged public flag must be omitted (nil pointer), not sent as false.
	if upd.IsPublic != nil {
		t.Errorf("unchanged public flag must be omitted, got %v", *upd.IsPublic)
	}
}

func TestApplyDatabaseDeleteUsesDefaultFlags(t *testing.T) {
	mc := &mockClient{}
	resolved := projectResolved()
	resolved[state.ResourceKey{Kind: resource.KindDatabase, Name: "mongo"}] = "db-uuid"
	ops := []apply.Operation{apply.DatabaseOp(apply.OpDelete, database("mongo", "mongodb"), nil)}
	if _, err := apply.NewEngine(mc, resolved, nil).Apply(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	if mc.calls[0].method != "DeleteDatabase" || mc.calls[0].uuid != "db-uuid" {
		t.Fatalf("want DeleteDatabase db-uuid, got %+v", mc.calls[0])
	}
	if mc.calls[0].delOpts != coolify.DefaultDeleteDatabaseOptions() {
		t.Errorf("delete must use the all-true default teardown flags, got %+v", mc.calls[0].delOpts)
	}
}

func TestApplyDatabaseDeleteAbsentIsNoop(t *testing.T) {
	mc := &mockClient{}
	// The database is not in the resolved map: a delete is a silent no-op.
	ops := []apply.Operation{apply.DatabaseOp(apply.OpDelete, database("gone", "postgresql"), nil)}
	if _, err := apply.NewEngine(mc, projectResolved(), nil).Apply(context.Background(), ops); err != nil {
		t.Fatal(err)
	}
	if len(mc.calls) != 0 {
		t.Errorf("deleting an absent database must make no client call, got %d", len(mc.calls))
	}
}

func TestDAGDatabaseDependsOnProjectAndEnvironment(t *testing.T) {
	// A database create must be ordered after its project and environment.
	ops, err := apply.OrderApply([]apply.Operation{
		apply.DatabaseOp(apply.OpCreate, database("pg", "postgresql"), nil),
		apply.CreateEnvironmentOp(resource.Environment{Metadata: resource.EnvironmentMeta{Name: "staging", Project: "beenaire"}}),
		apply.CreateProjectOp(resource.Project{Metadata: resource.ProjectMeta{Name: "beenaire"}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	order := make([]string, len(ops))
	for i, op := range ops {
		order[i] = op.Kind
	}
	if order[0] != resource.KindProject || order[1] != resource.KindEnvironment || order[2] != resource.KindDatabase {
		t.Errorf("apply order = %v, want Project, Environment, Database", order)
	}

	// Reverse order tears the database down before its environment and project.
	del, err := apply.OrderDelete([]apply.Operation{
		apply.DatabaseOp(apply.OpDelete, database("pg", "postgresql"), nil),
		apply.DeleteEnvironmentOp(resource.Environment{Metadata: resource.EnvironmentMeta{Name: "staging", Project: "beenaire"}}),
		apply.DeleteProjectOp(resource.Project{Metadata: resource.ProjectMeta{Name: "beenaire"}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if del[0].Kind != resource.KindDatabase {
		t.Errorf("delete order = %s first, want Database before its parents", del[0].Kind)
	}
}
