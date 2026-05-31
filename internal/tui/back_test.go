package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func escKey() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEsc} }

// expandedModel resolves the fake tree and expands the project root, so a stray collapse is
// observable as a drop in the number of visible rows.
func expandedModel(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t)
	m, _ = step(t, m, m.Init()()) // resolvedMsg
	m.tree.cursor = 0
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand project
	if got := len(m.tree.visible()); got <= 2 {
		t.Fatalf("setup: project did not expand (visible=%d)", got)
	}
	return m
}

func TestHandleBack_EscClosesLogsBeforeTree(t *testing.T) {
	m := expandedModel(t)
	before := len(m.tree.visible())
	m.showLogs = true

	m, _ = step(t, m, escKey())
	if m.showLogs {
		t.Error("esc did not close the log pane")
	}
	if got := len(m.tree.visible()); got != before {
		t.Errorf("tree collapsed under the log pane: visible %d → %d", before, got)
	}
}

func TestHandleBack_EscClosesDriftAndClearsIt(t *testing.T) {
	m := expandedModel(t)
	before := len(m.tree.visible())
	m.showDrift = true
	m.drift = &driftView{title: "app", note: "n"}

	m, _ = step(t, m, escKey())
	if m.showDrift {
		t.Error("esc did not close the drift pane")
	}
	if m.drift != nil {
		t.Error("esc must clear the cached drift (toggleDrift refetches on reopen)")
	}
	if got := len(m.tree.visible()); got != before {
		t.Errorf("tree collapsed under the drift pane: visible %d → %d", before, got)
	}
}

func TestHandleBack_EscClosesDetailWithoutDesiredEnvs(t *testing.T) {
	m := expandedModel(t)
	// A database detail carries struct fields but no desired env rows; before the fix esc fell
	// through to the tree and left it open.
	m.detail = ptr(databaseDetail(newFakeClient().db))
	if m.detail.hasDesiredEnvs() {
		t.Fatal("setup: database detail unexpectedly has desired envs")
	}

	m, _ = step(t, m, escKey())
	if m.detail != nil {
		t.Errorf("esc did not close the detail panel: %+v", m.detail)
	}
}

func TestHandleBack_EscClosesDetailWithDesiredEnvs(t *testing.T) {
	m := expandedModel(t)
	m.detail = &detail{
		kind:        "Application",
		env:         "staging",
		name:        "web",
		desiredEnvs: []desiredEnvRow{{name: "NODE_ENV", display: "production"}},
	}

	m, _ = step(t, m, escKey())
	if m.detail != nil {
		t.Errorf("esc did not close the detail panel with desired envs: %+v", m.detail)
	}
}

func TestHandleBack_EscCollapsesTreeWhenNothingOpen(t *testing.T) {
	m := expandedModel(t)
	before := len(m.tree.visible())

	m, _ = step(t, m, escKey())
	if got := len(m.tree.visible()); got >= before {
		t.Errorf("esc with no overlay must collapse the tree: visible %d → %d", before, got)
	}
}

// Desired-env up/down still moves the cursor after the Back case was removed from
// handleDesiredNav, and esc closes the detail rather than being swallowed by the env nav.
func TestHandleBack_DesiredCursorStillMovesThenEscCloses(t *testing.T) {
	m := newTestModel(t)
	m.detail = &detail{
		desiredEnvs: []desiredEnvRow{
			{name: "A", display: "1"},
			{name: "B", display: "2"},
		},
	}

	m, _ = step(t, m, keyRunes('j')) // down over the env rows
	if m.detail.envCursor != 1 {
		t.Fatalf("down did not move the env cursor: %d", m.detail.envCursor)
	}
	m, _ = step(t, m, escKey())
	if m.detail != nil {
		t.Error("esc must close the detail, not be swallowed by the env cursor nav")
	}
}

func TestHandleBack_EditAndConfirmEscUnchanged(t *testing.T) {
	// editing: esc cancels the edit, detail stays open.
	m := newTestModel(t)
	m.detail = &detail{desiredEnvs: []desiredEnvRow{{name: "A", display: "1"}}}
	m, _ = step(t, m, keyRunes('e')) // open edit
	if m.editing == nil {
		t.Fatal("setup: edit did not open")
	}
	m, _ = step(t, m, escKey())
	if m.editing != nil {
		t.Error("esc did not cancel the edit")
	}
	if m.detail == nil {
		t.Error("esc on an edit must not also close the detail")
	}

	// confirm: esc cancels the prompt.
	m2 := newTestModel(t)
	m2.confirm = &confirmState{prompt: "quit? [y/N]", onConfirm: tea.Quit}
	m2, _ = step(t, m2, escKey())
	if m2.confirm != nil {
		t.Error("esc did not cancel the confirmation prompt")
	}
}
