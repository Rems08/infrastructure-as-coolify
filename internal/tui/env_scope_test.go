package tui

import (
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

// envRows keeps scope variants of a key as distinct rows and collapses only exact (key, scope)
// duplicates.
func TestEnvRows_DedupeByKeyScope(t *testing.T) {
	rows := envRows([]coolify.ServiceEnvVar{
		{Key: "KEY", Value: "v", IsBuildtime: true},
		{Key: "KEY", Value: "v", IsRuntime: true},
		{Key: "OTHER", Value: "o", IsRuntime: true},
		{Key: "OTHER", Value: "o", IsRuntime: true},
	})
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (KEY×2 scope variants, OTHER collapsed)", len(rows))
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.keyLabel()] = r.value
	}
	for _, want := range []string{"KEY [build]", "KEY [runtime]", "OTHER [runtime]"} {
		if _, ok := got[want]; !ok {
			t.Errorf("missing row %q; got %+v", want, rows)
		}
	}
}

// Two entries sharing a key and a scope but disagreeing on the value collapse to one row that
// is flagged conflict — the value itself is never compared in the view.
func TestEnvRows_ConflictOnSameKeyScopeDifferentValue(t *testing.T) {
	rows := envRows([]coolify.ServiceEnvVar{
		{Key: "DUP", Value: "a", IsRuntime: true},
		{Key: "DUP", Value: "b", IsRuntime: true},
	})
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (same key+scope collapses)", len(rows))
	}
	if !rows[0].conflict {
		t.Error("collapsed duplicates with differing values must flag conflict")
	}
}

// The only-remote section renders scope variants as labelled rows and drops exact duplicates;
// the presence counter stays set-based per key.
func TestAppEnv_ScopeVariantsRenderedAndCountedByKey(t *testing.T) {
	m := appEnvModel(t)
	m, _ = step(t, m, appDetailMsg{
		app: coolify.Application{Name: "web"}, env: "staging", name: "web",
		remoteEnvs: []coolify.ServiceEnvVar{
			{Key: "NODE_ENV", Value: "prod", IsBuildtime: true, IsRuntime: true}, // tracked
			{Key: "CACHE", Value: "x", IsBuildtime: true},                        // only-remote build
			{Key: "CACHE", Value: "y", IsRuntime: true},                          // only-remote runtime
			{Key: "QUEUE", Value: "q", IsRuntime: true},                          // only-remote
			{Key: "QUEUE", Value: "q", IsRuntime: true},                          // exact dup, collapsed
		},
	})

	tracked, onlyLocal, onlyRemote := m.detail.envComparison()
	if tracked != 1 || onlyRemote != 2 {
		t.Fatalf("comparison = %d tracked / %d only-local / %d only-remote; want 1 tracked, 2 only-remote (by key)",
			tracked, onlyLocal, onlyRemote)
	}

	view := m.View()
	for _, want := range []string{"CACHE [build]", "CACHE [runtime]", "QUEUE [runtime]"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing scope-labelled row %q:\n%s", want, view)
		}
	}
	// QUEUE [runtime] appears once: the exact duplicate collapsed.
	if n := strings.Count(view, "QUEUE [runtime]"); n != 1 {
		t.Errorf("QUEUE [runtime] rendered %d times, want 1 (exact dup collapsed)", n)
	}
}
