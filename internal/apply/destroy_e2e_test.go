package apply_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// destroyResolved is the live state for a project+environment+application(+service) stack,
// keyed the way the resolver keys live resources.
func destroyResolved(withService bool) state.Map {
	m := state.Map{
		state.ResourceKey{Kind: resource.KindProject, Name: "beenaire"}:                                             "proj-uuid",
		state.ResourceKey{Project: "beenaire", Kind: resource.KindEnvironment, Name: "staging"}:                     "staging",
		state.ResourceKey{Project: "beenaire", Environment: "staging", Kind: resource.KindApplication, Name: "web"}: "app-uuid",
	}
	if withService {
		m[state.ResourceKey{Project: "beenaire", Environment: "staging", Kind: resource.KindService, Name: "cache"}] = "svc-uuid"
	}
	return m
}

func destroyInput(resolved state.Map, withService bool) apply.DeleteInput {
	in := apply.DeleteInput{
		Projects:     []resource.Project{proj("beenaire")},
		Environments: []resource.Environment{env("beenaire", "staging")},
		Applications: []resource.Application{app("beenaire", "staging", "web")},
		Resolved:     resolved,
	}
	if withService {
		in.Services = []resource.Service{{Metadata: resource.ServiceMeta{Project: "beenaire", Environment: "staging", Name: "cache"}}}
	}
	return in
}

// deletePaths returns the paths of the DELETE requests, in the order they were received.
func deletePaths(reqs []wroteReq) []string {
	var out []string
	for _, r := range reqs {
		if r.method == http.MethodDelete {
			out = append(out, r.path)
		}
	}
	return out
}

func indexOfPath(paths []string, want string) int {
	for i, p := range paths {
		if p == want {
			return i
		}
	}
	return -1
}

func TestDestroyE2E(t *testing.T) {
	t.Run("basic reverse order", func(t *testing.T) {
		srv, reqs := e2eServer(t, "")
		client := e2eClient(t, srv.URL)
		resolved := destroyResolved(false)
		ordered, err := apply.OrderDelete(destroyInput(resolved, false).DeleteOperations())
		if err != nil {
			t.Fatal(err)
		}
		eng := apply.NewEngine(client, resolved, nil)
		if _, err := eng.Apply(context.Background(), ordered); err != nil {
			t.Fatal(err)
		}
		paths := deletePaths(*reqs)
		appIdx := indexOfPath(paths, "/api/v1/applications/app-uuid")
		envIdx := indexOfPath(paths, "/api/v1/projects/proj-uuid/environments/staging")
		projIdx := indexOfPath(paths, "/api/v1/projects/proj-uuid")
		if appIdx < 0 || envIdx < 0 || projIdx < 0 {
			t.Fatalf("missing DELETEs in %v", paths)
		}
		if !(appIdx < envIdx && envIdx < projIdx) {
			t.Errorf("delete order = %v, want application < environment < project", paths)
		}
	})

	t.Run("service and app deleted before environment", func(t *testing.T) {
		srv, reqs := e2eServer(t, "")
		client := e2eClient(t, srv.URL)
		resolved := destroyResolved(true)
		ordered, err := apply.OrderDelete(destroyInput(resolved, true).DeleteOperations())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := apply.NewEngine(client, resolved, nil).Apply(context.Background(), ordered); err != nil {
			t.Fatal(err)
		}
		paths := deletePaths(*reqs)
		envIdx := indexOfPath(paths, "/api/v1/projects/proj-uuid/environments/staging")
		appIdx := indexOfPath(paths, "/api/v1/applications/app-uuid")
		svcIdx := indexOfPath(paths, "/api/v1/services/svc-uuid")
		if appIdx < 0 || svcIdx < 0 || envIdx < 0 {
			t.Fatalf("missing DELETEs in %v", paths)
		}
		if appIdx > envIdx || svcIdx > envIdx {
			t.Errorf("children must be deleted before their environment: %v", paths)
		}
	})

	t.Run("partial failure stops and audits applied deletes", func(t *testing.T) {
		// The project delete fails; the application and environment before it succeed.
		srv, reqs := e2eServer(t, "/api/v1/projects/proj-uuid")
		client := e2eClient(t, srv.URL)
		resolved := destroyResolved(false)
		ordered, err := apply.OrderDelete(destroyInput(resolved, false).DeleteOperations())
		if err != nil {
			t.Fatal(err)
		}
		auditPath := filepath.Join(t.TempDir(), "audit.log")
		eng := apply.NewEngine(client, resolved, apply.NewAuditor(auditPath))
		sum, err := eng.Apply(context.Background(), ordered)
		if err == nil {
			t.Fatal("expected the project delete to fail")
		}
		if sum.Applied != 2 {
			t.Errorf("Applied = %d, want 2 (application + environment before the failure)", sum.Applied)
		}
		data, rErr := os.ReadFile(auditPath)
		if rErr != nil {
			t.Fatal(rErr)
		}
		if lines := strings.Count(strings.TrimSpace(string(data)), "\n") + 1; lines != 2 {
			t.Errorf("audit log = %d entries, want 2 (only the succeeded deletes)", lines)
		}
		_ = reqs
	})

	t.Run("dry-run builds the plan with no HTTP call", func(t *testing.T) {
		srv, reqs := e2eServer(t, "")
		_ = e2eClient(t, srv.URL)
		// An offline dry-run assumes every declared resource is present and never calls the
		// API: it only builds and orders the operations.
		in := destroyInput(state.Map{}, false)
		in.AssumePresent = true
		ordered, err := apply.OrderDelete(in.DeleteOperations())
		if err != nil {
			t.Fatal(err)
		}
		if len(ordered) != 3 {
			t.Errorf("dry-run plan = %d ops, want 3", len(ordered))
		}
		if n := len(*reqs); n != 0 {
			t.Errorf("dry-run made %d HTTP requests, want 0", n)
		}
	})
}
