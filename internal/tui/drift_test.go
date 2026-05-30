package tui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
)

func TestDrift_NoConfigPathReportsUnavailable(t *testing.T) {
	m := newTestModel(t) // built without WithConfigPath
	m = selectApp(t, m)

	m, cmd := step(t, m, keyRunes('D'))
	if cmd == nil {
		t.Fatal("D did not return a drift command")
	}
	m, _ = step(t, m, cmd())
	if !m.showDrift || m.drift == nil {
		t.Fatal("drift pane not shown")
	}
	if !strings.Contains(m.View(), "drift unavailable") {
		t.Errorf("missing unavailable note:\n%s", m.View())
	}
}

func TestDrift_NameNotInConfigReportsUnavailable(t *testing.T) {
	// An empty config dir has no Application named "web" (the selected leaf), so drift is
	// reported unavailable rather than crashing.
	m := NewModel(context.Background(), newFakeClient(), newFakeClient(), WithConfigPath(t.TempDir()))
	m = selectApp(t, m)

	m, cmd := step(t, m, keyRunes('D'))
	m, _ = step(t, m, cmd())
	if !strings.Contains(m.View(), "drift unavailable") {
		t.Errorf("expected unavailable note for a missing desired app:\n%s", m.View())
	}
}

func TestDrift_ComputesChangesByName(t *testing.T) {
	m := NewModel(context.Background(), newFakeClient(), newFakeClient(),
		WithConfigPath(filepath.Join("testdata", "drift")))
	m = selectApp(t, m)

	m, cmd := step(t, m, keyRunes('D'))
	m, _ = step(t, m, cmd())
	if m.drift == nil || len(m.drift.changes) == 0 {
		t.Fatalf("expected drift changes, got %+v", m.drift)
	}
	view := m.View()
	// The desired fqdn differs from the live one (https://web.test): it must show as an update.
	if !strings.Contains(view, "fqdn") || !strings.Contains(view, "https://web.example.com") {
		t.Errorf("fqdn drift not rendered:\n%s", view)
	}
	if !strings.Contains(view, "~") {
		t.Errorf("update glyph missing:\n%s", view)
	}

	// D again toggles the pane closed.
	m, _ = step(t, m, keyRunes('D'))
	if m.showDrift {
		t.Error("second D did not close the drift pane")
	}
}

func TestRenderChange_RedactsSensitiveValue(t *testing.T) {
	leak := renderChange(plan.Change{
		Op: plan.OpUpdate, Path: "Application.web.env.SECRET",
		Old: "old-plaintext", New: "new-plaintext", Sensitive: true,
	})
	if strings.Contains(leak, "plaintext") {
		t.Fatalf("sensitive value leaked into drift render: %q", leak)
	}
	if !strings.Contains(leak, mask) {
		t.Errorf("sensitive change not masked: %q", leak)
	}

	safe := renderChange(plan.Change{
		Op: plan.OpUpdate, Path: "p", Old: "${env:OLD}", New: "${env:NEW}", Sensitive: true,
	})
	if !strings.Contains(safe, "${env:NEW}") {
		t.Errorf("safe source declaration was hidden: %q", safe)
	}
}
