package apply_test

import (
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

func proj(name string) resource.Project {
	return resource.Project{Metadata: resource.ProjectMeta{Name: name}}
}

func env(project, name string) resource.Environment {
	return resource.Environment{Metadata: resource.EnvironmentMeta{Project: project, Name: name}}
}

func app(project, environment, name string) resource.Application {
	return resource.Application{Metadata: resource.ApplicationMeta{Project: project, Environment: environment, Name: name}}
}

func destroyStack() apply.DeleteInput {
	return apply.DeleteInput{
		Projects:     []resource.Project{proj("beenaire")},
		Environments: []resource.Environment{env("beenaire", "staging")},
		Applications: []resource.Application{app("beenaire", "staging", "web")},
	}
}

func opKinds(ops []apply.Operation) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = op.Kind
	}
	return out
}

func TestDeleteOperationsSkipsAbsentRemote(t *testing.T) {
	in := destroyStack()
	in.Resolved = state.Map{} // nothing resolved → nothing exists remotely
	if ops := in.DeleteOperations(); len(ops) != 0 {
		t.Errorf("DeleteOperations on empty state = %d ops, want 0 (idempotent destroy)", len(ops))
	}
}

func TestDeleteOperationsAssumePresentOffline(t *testing.T) {
	in := destroyStack()
	in.Resolved = state.Map{}
	in.AssumePresent = true
	if ops := in.DeleteOperations(); len(ops) != 3 {
		t.Errorf("AssumePresent DeleteOperations = %d ops, want 3", len(ops))
	}
}

func TestDeleteOperationsOnlyResolved(t *testing.T) {
	in := destroyStack()
	in.Resolved = state.Map{
		state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}: "proj-uuid",
		// environment and application are NOT resolved → only the project is deleted.
	}
	ops := in.DeleteOperations()
	if len(ops) != 1 || ops[0].Kind != resource.KindProject {
		t.Errorf("DeleteOperations = %v, want a single Project op", opKinds(ops))
	}
}

func TestDeleteOperationsTargetFilter(t *testing.T) {
	in := destroyStack()
	in.AssumePresent = true
	in.Only = "web"
	ops := in.DeleteOperations()
	if len(ops) != 1 || ops[0].Kind != resource.KindApplication {
		t.Errorf("--target=web DeleteOperations = %v, want a single Application op", opKinds(ops))
	}
}

func TestDeleteOperationsReverseOrder(t *testing.T) {
	in := destroyStack()
	in.AssumePresent = true
	ordered, err := apply.OrderDelete(in.DeleteOperations())
	if err != nil {
		t.Fatal(err)
	}
	// Reverse dependency order: application before environment before project.
	idx := map[string]int{}
	for i, op := range ordered {
		idx[op.Kind] = i
	}
	if idx[resource.KindApplication] >= idx[resource.KindEnvironment] || idx[resource.KindEnvironment] >= idx[resource.KindProject] {
		t.Errorf("delete order = %v, want application < environment < project", opKinds(ordered))
	}
}
