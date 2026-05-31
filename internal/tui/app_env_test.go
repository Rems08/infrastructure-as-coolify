package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// appEnvModel builds a model whose desired index holds the testdata web application
// (NODE_ENV plain, DATABASE_URL secret) so the desired↔remote comparison can be exercised.
func appEnvModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("WEB_DB_URL", "postgres://example")
	msg, ok := loadDesiredCmd(desiredPath())().(desiredLoadedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("desired load failed: %+v", msg)
	}
	m := NewModel(context.Background(), newFakeClient(), newFakeClient(), WithConfigPath(desiredPath()))
	m.desired = msg.index
	m.loading = false // skip the resolve screen; these tests render the detail pane directly.
	return m
}

// webRemoteEnvs joins the testdata web app: NODE_ENV is tracked, DATABASE_URL stays only-local,
// API_KEY is only on the remote.
func webRemoteEnvs() []coolify.ServiceEnvVar {
	return []coolify.ServiceEnvVar{
		{Key: "NODE_ENV", Value: "production"},
		{Key: "API_KEY", Value: "xyz-remote"},
	}
}

// *coolify.Client still satisfies the read-only explorer after the new ListApplicationEnvs
// method was added to the interface.
var _ explorerClient = (*coolify.Client)(nil)

// loadDetailCmd lists an application's remote env vars and carries them on the message.
func TestLoadDetailCmd_AppFetchesRemoteEnvs(t *testing.T) {
	fc := newFakeClient()
	fc.appEnvs = webRemoteEnvs()
	node := &treeNode{label: "web", kind: resource.KindApplication, uuid: "a1"}
	node.key.Environment = "staging"

	msg, ok := loadDetailCmd(context.Background(), fc, node)().(appDetailMsg)
	if !ok {
		t.Fatalf("loadDetailCmd = %T, want appDetailMsg", msg)
	}
	if msg.remoteErr {
		t.Error("remoteErr set despite a successful env listing")
	}
	if len(msg.remoteEnvs) != 2 {
		t.Fatalf("remoteEnvs = %d, want 2", len(msg.remoteEnvs))
	}
}

// AC.E2a.1 — a failed env listing degrades gracefully: the detail still loads, the comparison is
// reported unavailable, and no error panel is raised.
func TestAppDetail_RemoteEnvFailureDegradesGracefully(t *testing.T) {
	fc := newFakeClient()
	fc.appEnvs = webRemoteEnvs()
	fc.appEnvsErr = errors.New("envs down")
	node := &treeNode{label: "web", kind: resource.KindApplication, uuid: "a1"}
	node.key.Environment = "staging"

	msg, ok := loadDetailCmd(context.Background(), fc, node)().(appDetailMsg)
	if !ok {
		t.Fatalf("loadDetailCmd = %T, want appDetailMsg (not errMsg)", msg)
	}
	if !msg.remoteErr {
		t.Fatal("remoteErr not set on a failed env listing")
	}

	m := appEnvModel(t)
	m, _ = step(t, m, msg)
	if m.err != nil {
		t.Errorf("env failure raised an error panel: %v", m.err)
	}
	if m.detail == nil || !m.detail.hasDesiredEnvs() {
		t.Fatal("detail or desired section dropped on a degraded load")
	}
	view := m.View()
	if !strings.Contains(view, "remote env unavailable") {
		t.Errorf("missing unavailable note:\n%s", view)
	}
	if strings.Contains(view, "only on remote") {
		t.Errorf("comparison rendered despite unavailable remote:\n%s", view)
	}
}

// AC.E2a.2 — desired and remote are joined by presence: tracked, only-local, only-remote.
func TestAppEnvComparison_ByPresence(t *testing.T) {
	m := appEnvModel(t)
	m, _ = step(t, m, appDetailMsg{
		app: coolify.Application{Name: "web"}, env: "staging", name: "web",
		remoteEnvs: webRemoteEnvs(),
	})

	tracked, onlyLocal, onlyRemote := m.detail.envComparison()
	if tracked != 1 || onlyLocal != 1 || onlyRemote != 1 {
		t.Fatalf("comparison = %d tracked, %d only-local, %d only-remote; want 1/1/1",
			tracked, onlyLocal, onlyRemote)
	}
	only := m.detail.onlyRemoteEnvs()
	if len(only) != 1 || only[0].key != "API_KEY" {
		t.Fatalf("onlyRemoteEnvs = %+v, want [API_KEY]", only)
	}

	view := m.View()
	for _, want := range []string{"1 tracked · 1 only-local · 1 only-remote", "only on remote", "API_KEY", "tracked", "only-local", "only-remote"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

// AC.E2a.2 — remote values are masked until revealed; a desired secret is never resolved.
func TestAppEnvComparison_RemoteMaskedSecretSafe(t *testing.T) {
	m := appEnvModel(t)
	m, _ = step(t, m, appDetailMsg{
		app: coolify.Application{Name: "web"}, env: "staging", name: "web",
		remoteEnvs: webRemoteEnvs(),
	})

	masked := m.View()
	if strings.Contains(masked, "xyz-remote") {
		t.Errorf("remote value shown before reveal:\n%s", masked)
	}
	if !strings.Contains(masked, mask) {
		t.Errorf("remote value not masked:\n%s", masked)
	}
	if !strings.Contains(masked, "${env:WEB_DB_URL}") {
		t.Errorf("secret source not shown:\n%s", masked)
	}

	m, _ = step(t, m, keyRunes('r'))
	revealed := m.View()
	if !strings.Contains(revealed, "xyz-remote") {
		t.Errorf("remote value not revealed after r:\n%s", revealed)
	}
	// Reveal never resolves a secret: the source stays, the value never appears.
	if strings.Contains(revealed, "postgres://example") {
		t.Fatalf("reveal leaked the desired secret value:\n%s", revealed)
	}
	if !strings.Contains(revealed, "${env:WEB_DB_URL}") {
		t.Errorf("secret source lost after reveal:\n%s", revealed)
	}
}

// AC.E2a.2 — e edits the cursored desired row (the β flow is preserved) and the cursor never
// reaches an only-remote row, which is read-only.
func TestAppEnvComparison_EditDesiredOnlyRemoteReadOnly(t *testing.T) {
	m := appEnvModel(t)
	m, _ = step(t, m, appDetailMsg{
		app: coolify.Application{Name: "web"}, env: "staging", name: "web",
		remoteEnvs: webRemoteEnvs(),
	})

	// Three env vars are visible (2 desired + 1 only-remote), but the cursor clamps to the two
	// desired rows: an only-remote row is never selectable.
	for i := 0; i < 5; i++ {
		m, _ = step(t, m, keyRunes('j'))
	}
	if max := len(m.detail.desiredEnvs) - 1; m.detail.envCursor != max {
		t.Fatalf("envCursor = %d, want clamped at %d (only-remote not selectable)", m.detail.envCursor, max)
	}

	m, _ = step(t, m, keyRunes('e'))
	if m.editing == nil {
		t.Fatal("e did not open an edit on the cursored desired row")
	}
	if m.editing.name != "DATABASE_URL" {
		t.Errorf("editing %q, want the cursored desired row DATABASE_URL", m.editing.name)
	}
}
