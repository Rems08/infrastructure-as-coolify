package apply

import (
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func proj(name string) Operation {
	return CreateProjectOp(resource.Project{Metadata: resource.ProjectMeta{Name: name}})
}

func env(project, name string) Operation {
	return CreateEnvironmentOp(resource.Environment{Metadata: resource.EnvironmentMeta{Name: name, Project: project}})
}

func app(project, environment, name string) Operation {
	return ApplicationOp(OpCreate, resource.Application{
		Metadata: resource.ApplicationMeta{Name: name, Project: project, Environment: environment},
	}, nil)
}

// indexOf returns the position of the first op matching kind+name, or -1.
func indexOf(ops []Operation, kind, name string) int {
	for i, op := range ops {
		if op.Kind == kind && op.Name == name {
			return i
		}
	}
	return -1
}

func TestOrderApplyLinear(t *testing.T) {
	// Deliberately unsorted input: app, project, env.
	in := []Operation{app("p", "staging", "a"), proj("p"), env("p", "staging")}
	out, err := OrderApply(in)
	if err != nil {
		t.Fatal(err)
	}
	pi := indexOf(out, resource.KindProject, "p")
	ei := indexOf(out, resource.KindEnvironment, "staging")
	ai := indexOf(out, resource.KindApplication, "a")
	if !(pi < ei && ei < ai) {
		t.Errorf("want project(%d) < environment(%d) < application(%d)", pi, ei, ai)
	}
}

func TestOrderApplyBranch(t *testing.T) {
	in := []Operation{
		app("p", "production", "api"),
		app("p", "staging", "api"),
		env("p", "staging"),
		env("p", "production"),
		proj("p"),
	}
	out, err := OrderApply(in)
	if err != nil {
		t.Fatal(err)
	}
	pi := indexOf(out, resource.KindProject, "p")
	// Every environment and application must come after the project.
	for _, op := range out {
		idx := indexOf(out, op.Kind, op.Name)
		if op.Kind != resource.KindProject && idx < pi {
			t.Errorf("%s %s at %d precedes its project at %d", op.Kind, op.Name, idx, pi)
		}
	}
	if len(out) != 5 {
		t.Errorf("got %d ops, want 5", len(out))
	}
}

func TestOrderApplyOrphans(t *testing.T) {
	// An application whose project/environment are not part of this run: no edges, but it
	// must still be returned (its parents already exist remotely).
	in := []Operation{app("already", "there", "x")}
	out, err := OrderApply(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || indexOf(out, resource.KindApplication, "x") != 0 {
		t.Errorf("orphan application must be returned, got %+v", out)
	}
}

func TestOrderDeleteReverse(t *testing.T) {
	in := []Operation{proj("p"), env("p", "staging"), app("p", "staging", "a")}
	out, err := OrderDelete(in)
	if err != nil {
		t.Fatal(err)
	}
	pi := indexOf(out, resource.KindProject, "p")
	ei := indexOf(out, resource.KindEnvironment, "staging")
	ai := indexOf(out, resource.KindApplication, "a")
	if !(ai < ei && ei < pi) {
		t.Errorf("delete order must be reverse: application(%d) < environment(%d) < project(%d)", ai, ei, pi)
	}
}

func TestTopoSortCycleDetected(t *testing.T) {
	nodes := []string{"A", "B"}
	deps := map[string][]string{"A": {"B"}, "B": {"A"}}
	if _, err := topoSort(nodes, deps); err == nil {
		t.Fatal("a cycle A->B->A must be detected")
	}
}

func TestTopoSortIgnoresAbsentDependency(t *testing.T) {
	// A depends on a node not present in the set; it should still be ordered, not dropped.
	out, err := topoSort([]string{"A"}, map[string][]string{"A": {"ghost"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0] != "A" {
		t.Errorf("out = %v, want [A]", out)
	}
}
