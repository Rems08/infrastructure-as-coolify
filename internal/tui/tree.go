package tui

import (
	"sort"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// kindGroup labels a synthetic node that has no Coolify counterpart (it only groups
// children in the view, e.g. the databases the resolver keys by name alone).
const kindGroup = "Group"

// treeNode is one row of the browser hierarchy: a project, an environment, a synthetic
// group, or a leaf resource (application/service/database). Only leaf resources carry a
// uuid the detail view can fetch.
type treeNode struct {
	label    string
	kind     string
	key      state.ResourceKey
	uuid     string
	children []*treeNode
	expanded bool
}

// isLeaf reports whether the node is a fetchable resource rather than a container.
func (n *treeNode) isLeaf() bool { return len(n.children) == 0 && n.kind != kindGroup }

// buildTree turns a resolved UUID map into the project → environment → resource hierarchy.
// Applications and services carry their (project, environment) coordinates and nest under
// the matching environment. Databases the resolver keys by name alone (no environment_id;
// see state.Resolve) cannot be placed in the hierarchy, so they are grouped under a single
// top-level "databases" node rather than invented into a project they may not belong to.
func buildTree(m state.Map) []*treeNode {
	projects := map[string]*treeNode{}
	envs := map[string]*treeNode{} // keyed by project + "/" + env name
	var unscopedDBs []*treeNode

	for key, uuid := range m {
		if key.Kind == resource.KindProject {
			projects[key.Name] = &treeNode{label: key.Name, kind: key.Kind, key: key, uuid: uuid}
		}
	}
	for key := range m {
		if key.Kind == resource.KindEnvironment {
			p := ensureProject(projects, key.Project)
			node := &treeNode{label: key.Name, kind: key.Kind, key: key}
			envs[key.Project+"/"+key.Name] = node
			p.children = append(p.children, node)
		}
	}
	for key, uuid := range m {
		switch key.Kind {
		case resource.KindApplication, resource.KindService:
			leaf := &treeNode{label: key.Name, kind: key.Kind, key: key, uuid: uuid}
			if env, ok := envs[key.Project+"/"+key.Environment]; ok {
				env.children = append(env.children, leaf)
			}
		case resource.KindDatabase:
			unscopedDBs = append(unscopedDBs, &treeNode{label: key.Name, kind: key.Kind, key: key, uuid: uuid})
		}
	}

	roots := make([]*treeNode, 0, len(projects)+1)
	for _, p := range projects {
		sortChildren(p)
		roots = append(roots, p)
	}
	sortNodes(roots)
	if len(unscopedDBs) > 0 {
		sortNodes(unscopedDBs)
		roots = append(roots, &treeNode{label: "databases", kind: kindGroup, children: unscopedDBs})
	}
	return roots
}

// ensureProject returns the project node for name, creating a placeholder when the map
// resolved an environment whose project was not itself listed (defensive; Resolve keys
// both, so this is belt-and-braces).
func ensureProject(projects map[string]*treeNode, name string) *treeNode {
	if p, ok := projects[name]; ok {
		return p
	}
	p := &treeNode{label: name, kind: resource.KindProject, key: state.ResourceKey{Kind: resource.KindProject, Name: name}}
	projects[name] = p
	return p
}

func sortChildren(n *treeNode) {
	sortNodes(n.children)
	for _, c := range n.children {
		sortChildren(c)
	}
}

func sortNodes(nodes []*treeNode) {
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].label < nodes[j].label })
}

// visibleRow is a node paired with its indentation depth for rendering and navigation.
type visibleRow struct {
	node  *treeNode
	depth int
}

// tree holds the browsable hierarchy and the cursor over its visible rows. Navigation is
// pure (no I/O), so it is unit-tested without a running program.
type tree struct {
	roots  []*treeNode
	cursor int
}

// visible flattens the roots into the rows currently shown, descending into expanded nodes
// only.
func (t *tree) visible() []visibleRow {
	var rows []visibleRow
	var walk func(nodes []*treeNode, depth int)
	walk = func(nodes []*treeNode, depth int) {
		for _, n := range nodes {
			rows = append(rows, visibleRow{node: n, depth: depth})
			if n.expanded {
				walk(n.children, depth+1)
			}
		}
	}
	walk(t.roots, 0)
	return rows
}

// selected returns the node under the cursor, or nil when the tree is empty.
func (t *tree) selected() *treeNode {
	rows := t.visible()
	if t.cursor < 0 || t.cursor >= len(rows) {
		return nil
	}
	return rows[t.cursor].node
}

func (t *tree) down() {
	if n := len(t.visible()); t.cursor < n-1 {
		t.cursor++
	}
}

func (t *tree) up() {
	if t.cursor > 0 {
		t.cursor--
	}
}

// toggle expands or collapses the selected container. It reports whether the node is a
// fetchable leaf, so the caller can trigger a detail load instead.
func (t *tree) toggle() (leaf *treeNode) {
	n := t.selected()
	if n == nil {
		return nil
	}
	if n.isLeaf() {
		return n
	}
	n.expanded = !n.expanded
	return nil
}

// collapse closes the selected node, or moves the cursor to its parent when the node is
// already collapsed (a leaf or a closed container).
func (t *tree) collapse() {
	n := t.selected()
	if n == nil {
		return
	}
	if len(n.children) > 0 && n.expanded {
		n.expanded = false
		return
	}
	t.cursorToParent()
}

// cursorToParent walks the visible rows backwards to the nearest row shallower than the
// current one.
func (t *tree) cursorToParent() {
	rows := t.visible()
	if t.cursor <= 0 || t.cursor >= len(rows) {
		return
	}
	depth := rows[t.cursor].depth
	for i := t.cursor - 1; i >= 0; i-- {
		if rows[i].depth < depth {
			t.cursor = i
			return
		}
	}
}
