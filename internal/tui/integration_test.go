package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestIntegration_DriftThenRestart drives a full session through the message loop on fakes:
// resolve, select an application, view its drift, close it, restart it through the confirm
// modal, and quit — asserting state and side effects at each step without a real terminal.
func TestIntegration_DriftThenRestart(t *testing.T) {
	fc := newFakeClient()
	rec := &fakeRecorder{}
	m := NewModel(context.Background(), fc, fc,
		WithConfigPath(filepath.Join("testdata", "drift")),
		WithAuditor(rec),
	)
	m = selectApp(t, m)

	// Drift: compute and show.
	m, cmd := step(t, m, keyRunes('D'))
	m, _ = step(t, m, cmd())
	if !m.showDrift || m.drift == nil || len(m.drift.changes) == 0 {
		t.Fatalf("drift not computed in session: %+v", m.drift)
	}
	if !strings.Contains(m.View(), "drift  web") {
		t.Errorf("drift pane not rendered:\n%s", m.View())
	}
	// Close it.
	m, _ = step(t, m, keyRunes('D'))
	if m.showDrift {
		t.Fatal("drift pane not closed on second D")
	}

	// Restart behind the confirm modal.
	m, _ = step(t, m, keyRunes('R'))
	if m.confirm == nil {
		t.Fatal("restart did not arm a confirm prompt")
	}
	m, cmd = step(t, m, keyRunes('y'))
	if cmd == nil {
		t.Fatal("confirm returned no lifecycle command")
	}
	m, _ = step(t, m, cmd()) // run the mutation, feed the resulting mutationDoneMsg

	if len(fc.lifecycle) != 1 || fc.lifecycle[0] != "restart:a1" {
		t.Fatalf("restart not performed: %v", fc.lifecycle)
	}
	if m.status == "" {
		t.Fatal("no success status after restart")
	}
	if len(rec.entries) != 1 || rec.entries[0].Operation != "restart" {
		t.Fatalf("restart not audited: %+v", rec.entries)
	}

	// Quit cleanly.
	_, qcmd := step(t, m, keyRunes('q'))
	if _, ok := qcmd().(tea.QuitMsg); !ok {
		t.Fatal("session did not quit")
	}
}

// TestIntegration_DesiredEnvInDetail drives a session that resolves, loads the desired index,
// opens the web application, and asserts its detail shows the desired env vars leak-proof: a
// plain value masked until revealed, a secret by its source declaration only.
func TestIntegration_DesiredEnvInDetail(t *testing.T) {
	t.Setenv("WEB_DB_URL", "postgres://example")
	fc := newFakeClient()
	m := NewModel(context.Background(), fc, fc, WithConfigPath(desiredPath()))

	m = selectApp(t, m)                                // resolve + park on web
	m, _ = step(t, m, loadDesiredCmd(desiredPath())()) // populate the desired index
	if _, ok := m.desiredFor("staging", "web"); !ok {
		t.Fatal("desired index not loaded")
	}

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // open web
	if cmd == nil {
		t.Fatal("opening web returned no detail command")
	}
	m, _ = step(t, m, cmd()) // appDetailMsg → attach desired

	if m.detail == nil || !m.detail.hasDesiredEnvs() {
		t.Fatalf("desired env section not shown for web: %+v", m.detail)
	}
	view := m.View()
	if !strings.Contains(view, "${env:WEB_DB_URL}") {
		t.Errorf("secret source declaration not shown:\n%s", view)
	}
	if strings.Contains(view, "postgres://example") {
		t.Fatalf("resolved secret value leaked into detail:\n%s", view)
	}
	if strings.Contains(view, "production") {
		t.Errorf("plain value shown before reveal:\n%s", view)
	}
	m, _ = step(t, m, keyRunes('r'))
	if !strings.Contains(m.View(), "production") {
		t.Errorf("plain value not revealed after r:\n%s", m.View())
	}
}

// TestIntegration_NoDesiredConfigNote opens an application when no desired config matches and
// asserts the detail reports it gracefully rather than crashing or inventing a match.
func TestIntegration_NoDesiredConfigNote(t *testing.T) {
	fc := newFakeClient()
	m := NewModel(context.Background(), fc, fc, WithConfigPath(t.TempDir()))

	m = selectApp(t, m)
	m, _ = step(t, m, loadDesiredCmd(t.TempDir())()) // empty dir → empty index

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // open web
	m, _ = step(t, m, cmd())

	if m.detail == nil || m.detail.hasDesiredEnvs() {
		t.Fatal("unmatched application must carry no desired env rows")
	}
	if !strings.Contains(m.View(), "no desired config for this application") {
		t.Errorf("missing no-desired-config note:\n%s", m.View())
	}
}
