package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

// webManyRemoteEnvs returns NODE_ENV (tracked) plus 20 only-remote keys, three of which
// contain "db", so the list overflows the window (for scroll) and a key substring filters it.
func webManyRemoteEnvs() []coolify.ServiceEnvVar {
	envs := []coolify.ServiceEnvVar{{Key: "NODE_ENV", Value: "prod", IsRuntime: true}}
	for i := 0; i < 17; i++ {
		envs = append(envs, coolify.ServiceEnvVar{Key: fmt.Sprintf("SVC_%02d", i), Value: "v", IsRuntime: true})
	}
	for _, k := range []string{"DB_HOST", "DB_PORT", "CACHE_DB"} {
		envs = append(envs, coolify.ServiceEnvVar{Key: k, Value: "secret-" + k, IsRuntime: true})
	}
	return envs
}

func appWithManyRemote(t *testing.T) Model {
	t.Helper()
	m := appEnvModel(t)
	m, _ = step(t, m, appDetailMsg{
		app: coolify.Application{Name: "web"}, env: "staging", name: "web",
		remoteEnvs: webManyRemoteEnvs(),
	})
	return m
}

// / opens a filter on env keys; applying "db" narrows the only-remote list to the matching
// keys with an n/m indicator, and esc clears the filter without closing the detail.
func TestEnvFilter_ByKeyThenEscClears(t *testing.T) {
	m := appWithManyRemote(t)

	m, _ = step(t, m, keyRunes('/'))
	if m.filtering == nil {
		t.Fatal("/ did not open the filter input")
	}
	m, _ = step(t, m, keyRunes('d'))
	m, _ = step(t, m, keyRunes('b'))
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.filter != "db" {
		t.Fatalf("filter = %q, want db", m.filter)
	}
	view := m.View()
	if !strings.Contains(view, "filter: db (3/20)") {
		t.Errorf("missing filter indicator (3/20):\n%s", view)
	}
	for _, want := range []string{"DB_HOST", "DB_PORT", "CACHE_DB"} {
		if !strings.Contains(view, want) {
			t.Errorf("filtered view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "SVC_00") {
		t.Errorf("non-matching key SVC_00 shown under filter db:\n%s", view)
	}

	m, _ = step(t, m, escKey())
	if m.filter != "" {
		t.Errorf("esc did not clear the filter, filter = %q", m.filter)
	}
	if m.detail == nil {
		t.Error("esc closed the detail instead of just the filter")
	}
}

// A only-remote list longer than the window scrolls: down runs the cursor past the desired rows
// then scrolls, bounded at the end; up scrolls back to the top. No offset escapes [0, max].
func TestEnvScroll_BoundedAndAllReachable(t *testing.T) {
	m := appWithManyRemote(t)
	max := m.detail.maxEnvScroll("")
	if max != 20-envWindow {
		t.Fatalf("maxEnvScroll = %d, want %d", max, 20-envWindow)
	}

	// Top of the list is visible, end is not: a "↓ more" indicator, no "↑ more".
	top := m.View()
	if !strings.Contains(top, fmt.Sprintf("↓ %d more", max)) || strings.Contains(top, "↑ ") {
		t.Errorf("at the top the list must show a down indicator only:\n%s", top)
	}

	// Press down well past the end: the offset clamps at max, never beyond.
	for i := 0; i < 40; i++ {
		m, _ = step(t, m, keyRunes('j'))
	}
	if m.detail.envScroll != max {
		t.Fatalf("envScroll = %d after scrolling down, want clamped at %d", m.detail.envScroll, max)
	}
	bottom := m.View()
	if !strings.Contains(bottom, fmt.Sprintf("↑ %d more", max)) || strings.Contains(bottom, "↓ ") {
		t.Errorf("at the end the list must show an up indicator only:\n%s", bottom)
	}
	// The last only-remote key is reachable at the bottom of the scroll.
	if !strings.Contains(bottom, "CACHE_DB") {
		t.Errorf("last key unreachable at max scroll:\n%s", bottom)
	}

	// Press up well past the top: the offset clamps at 0.
	for i := 0; i < 40; i++ {
		m, _ = step(t, m, keyRunes('k'))
	}
	if m.detail.envScroll != 0 {
		t.Errorf("envScroll = %d after scrolling up, want 0", m.detail.envScroll)
	}
}

// A filter and a reveal compose: revealing shows the values of the filtered rows while the
// filter stays applied.
func TestEnvFilter_RevealKeepsFilter(t *testing.T) {
	m := appWithManyRemote(t)
	m.filter = "db"

	m, _ = step(t, m, keyRunes('r'))
	view := m.View()
	if !strings.Contains(view, "secret-DB_HOST") {
		t.Errorf("reveal did not show the filtered row value:\n%s", view)
	}
	if m.filter != "db" || !strings.Contains(view, "filter: db") {
		t.Errorf("reveal dropped the active filter, filter = %q:\n%s", m.filter, view)
	}
	if strings.Contains(view, "SVC_00") {
		t.Errorf("reveal widened the filter to non-matching keys:\n%s", view)
	}
}
