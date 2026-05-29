package apply

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// kindRank orders resource kinds by dependency depth: a higher rank depends on lower
// ranks existing first (project → environment → application).
func kindRank(kind string) int {
	switch kind {
	case resource.KindProject:
		return 0
	case resource.KindEnvironment:
		return 1
	default: // Application, Service, Database
		return 2
	}
}

// nodeID is an operation target's unique identity. The leading rank makes a lexical sort
// of node IDs respect the kind hierarchy, so a no-edge run still orders project→env→app.
func nodeID(kind, project, environment, name string) string {
	return strconv.Itoa(kindRank(kind)) + "|" + kind + "|" + project + "|" + environment + "|" + name
}

// dependsOn returns the node IDs an operation's target requires to already exist: an
// environment needs its project; an application needs its project and environment.
func dependsOn(op Operation) []string {
	switch op.Kind {
	case resource.KindEnvironment:
		return []string{nodeID(resource.KindProject, "", "", op.Project)}
	case resource.KindApplication, resource.KindService, resource.KindDatabase:
		return []string{
			nodeID(resource.KindProject, "", "", op.Project),
			nodeID(resource.KindEnvironment, op.Project, "", op.Environment),
		}
	default:
		return nil
	}
}

// OrderApply returns ops sorted so each resource is applied after the resources it depends
// on (project → environment → application). It errors on a dependency cycle.
func OrderApply(ops []Operation) ([]Operation, error) {
	return orderOps(ops, false)
}

// OrderDelete returns ops in reverse dependency order (application → environment →
// project), the safe order for deletion.
func OrderDelete(ops []Operation) ([]Operation, error) {
	return orderOps(ops, true)
}

func orderOps(ops []Operation, reverse bool) ([]Operation, error) {
	byID := make(map[string]Operation, len(ops))
	deps := make(map[string][]string, len(ops))
	nodes := make([]string, 0, len(ops))
	for _, op := range ops {
		id := nodeID(op.Kind, op.Project, op.Environment, op.Name)
		byID[id] = op
		deps[id] = dependsOn(op)
		nodes = append(nodes, id)
	}
	sorted, err := topoSort(nodes, deps)
	if err != nil {
		return nil, err
	}
	out := make([]Operation, len(sorted))
	for i, id := range sorted {
		idx := i
		if reverse {
			idx = len(sorted) - 1 - i
		}
		out[idx] = byID[id]
	}
	return out, nil
}

// topoSort returns nodes in dependency order (a node after every node it depends on) using
// Kahn's algorithm. Dependencies on nodes absent from the set are ignored. Ties are broken
// by node ID for a deterministic order. It errors when a cycle leaves nodes unprocessable.
func topoSort(nodes []string, deps map[string][]string) ([]string, error) {
	present := make(map[string]bool, len(nodes))
	indeg := make(map[string]int, len(nodes))
	for _, n := range nodes {
		present[n] = true
		indeg[n] = 0
	}
	children := make(map[string][]string)
	for n, ds := range deps {
		for _, d := range ds {
			if !present[d] {
				continue
			}
			children[d] = append(children[d], n)
			indeg[n]++
		}
	}

	var ready []string
	for _, n := range nodes {
		if indeg[n] == 0 {
			ready = append(ready, n)
		}
	}
	sort.Strings(ready)

	out := make([]string, 0, len(nodes))
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		out = append(out, n)
		for _, c := range children[n] {
			indeg[c]--
			if indeg[c] == 0 {
				ready = append(ready, c)
			}
		}
		sort.Strings(ready)
	}

	if len(out) != len(nodes) {
		return nil, fmt.Errorf("apply: dependency cycle detected among %s", remaining(indeg, out))
	}
	return out, nil
}

// remaining lists the nodes a cycle left unprocessed, for the error message.
func remaining(indeg map[string]int, processed []string) []string {
	done := make(map[string]bool, len(processed))
	for _, n := range processed {
		done[n] = true
	}
	var out []string
	for n := range indeg {
		if !done[n] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}
