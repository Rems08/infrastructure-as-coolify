package tui

import (
	"context"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

func resolvedFake(t *testing.T) state.Map {
	t.Helper()
	m, err := state.Resolve(context.Background(), newFakeClient())
	if err != nil {
		t.Fatalf("resolve fake: %v", err)
	}
	return m
}

func TestBuildTree_Hierarchy(t *testing.T) {
	roots := buildTree(resolvedFake(t))

	// restaurant-core project plus the unscoped databases group.
	if len(roots) != 2 {
		t.Fatalf("roots = %d, want 2 (project + databases group)", len(roots))
	}
	proj := roots[0]
	if proj.kind != resource.KindProject || proj.label != "restaurant-core" {
		t.Fatalf("root[0] = %q/%q, want Project/restaurant-core", proj.kind, proj.label)
	}
	if len(proj.children) != 1 || proj.children[0].label != "staging" {
		t.Fatalf("project children = %+v, want one env staging", proj.children)
	}
	env := proj.children[0]
	if got := len(env.children); got != 2 {
		t.Fatalf("env children = %d, want 2 (app + service)", got)
	}
	// Children are sorted by label: kafka (service) before web (application).
	if env.children[0].label != "kafka" || env.children[1].label != "web" {
		t.Fatalf("env children order = %q,%q, want kafka,web", env.children[0].label, env.children[1].label)
	}

	group := roots[1]
	if group.kind != kindGroup || len(group.children) != 1 || group.children[0].label != "redis" {
		t.Fatalf("databases group = %+v, want one db redis", group)
	}
	if group.children[0].uuid != "db1" {
		t.Errorf("redis uuid = %q, want db1", group.children[0].uuid)
	}
}

func TestTree_NavigationAndExpand(t *testing.T) {
	tr := tree{roots: buildTree(resolvedFake(t))}

	// Only two roots visible while collapsed.
	if got := len(tr.visible()); got != 2 {
		t.Fatalf("collapsed visible = %d, want 2", got)
	}
	// Project is a container: toggle expands it, no leaf returned.
	if leaf := tr.toggle(); leaf != nil {
		t.Fatalf("toggle project returned leaf %v, want expand", leaf)
	}
	if got := len(tr.visible()); got != 3 { // project, staging, databases group
		t.Fatalf("after expand visible = %d, want 3", got)
	}

	// Move to staging and expand it.
	tr.down()
	if leaf := tr.toggle(); leaf != nil {
		t.Fatalf("toggle env returned leaf %v, want expand", leaf)
	}
	if got := len(tr.visible()); got != 5 { // project, staging, kafka, web, databases group
		t.Fatalf("after env expand visible = %d, want 5", got)
	}

	// Move to kafka (service leaf) and open it.
	tr.down()
	leaf := tr.toggle()
	if leaf == nil || leaf.label != "kafka" {
		t.Fatalf("toggle on kafka = %v, want kafka leaf", leaf)
	}
}

func TestTree_CollapseMovesToParent(t *testing.T) {
	tr := tree{roots: buildTree(resolvedFake(t))}
	tr.toggle() // expand project
	tr.down()   // staging
	tr.toggle() // expand staging
	tr.down()   // kafka
	tr.down()   // web (leaf, collapsed)

	// Collapse on a leaf jumps the cursor to its parent (staging).
	tr.collapse()
	if got := tr.selected().label; got != "staging" {
		t.Fatalf("after collapse on leaf, selected = %q, want staging", got)
	}
	// Collapse again closes staging.
	tr.collapse()
	if tr.selected().expanded {
		t.Fatal("staging still expanded after collapse")
	}
}

func TestTree_UpDownBounds(t *testing.T) {
	tr := tree{roots: buildTree(resolvedFake(t))}
	tr.up() // already at top, no-op
	if tr.cursor != 0 {
		t.Fatalf("cursor = %d after up at top, want 0", tr.cursor)
	}
	for i := 0; i < 10; i++ {
		tr.down()
	}
	if tr.cursor != len(tr.visible())-1 {
		t.Fatalf("cursor = %d, want last visible %d", tr.cursor, len(tr.visible())-1)
	}
}
