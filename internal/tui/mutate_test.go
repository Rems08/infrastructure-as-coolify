package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// selectApp resolves the tree and parks the cursor on the "web" application leaf.
func selectApp(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = step(t, m, m.Init()()) // resolvedMsg
	m.tree.cursor = 0
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand project
	m, _ = step(t, m, keyRunes('j'))                  // staging
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand staging
	m, _ = step(t, m, keyRunes('j'))                  // kafka (service)
	m, _ = step(t, m, keyRunes('j'))                  // web (application)
	if n := m.tree.selected(); n == nil || n.kind != resource.KindApplication {
		t.Fatalf("expected an application selected, got %+v", n)
	}
	return m
}

func TestLifecycle_ConfirmRunsMutationAndTraces(t *testing.T) {
	mut := newFakeClient()
	rec := &fakeRecorder{}
	m := NewModel(context.Background(), newFakeClient(), mut, WithAuditor(rec))
	m = selectApp(t, m)

	// R arms the prompt; nothing runs yet.
	m, cmd := step(t, m, keyRunes('R'))
	if m.confirm == nil {
		t.Fatal("R did not arm a confirm prompt")
	}
	if cmd != nil {
		t.Fatal("arming a confirm must not run a command (no inline mutation)")
	}
	if len(mut.lifecycle) != 0 {
		t.Fatalf("mutation ran before confirmation: %v", mut.lifecycle)
	}

	// y confirms and returns the lifecycle command; the mutation runs only when it executes.
	m, cmd = step(t, m, keyRunes('y'))
	if m.confirm != nil {
		t.Fatal("confirm not cleared after y")
	}
	if cmd == nil {
		t.Fatal("confirm did not return the lifecycle command")
	}
	msg := cmd()
	if len(mut.lifecycle) != 1 || mut.lifecycle[0] != "restart:a1" {
		t.Fatalf("lifecycle = %v, want [restart:a1]", mut.lifecycle)
	}
	done, ok := msg.(mutationDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("msg = %#v, want successful mutationDoneMsg", msg)
	}

	// The action is traced in the append-only audit log, by identity, with no secret.
	if len(rec.entries) != 1 {
		t.Fatalf("audit entries = %+v, want one", rec.entries)
	}
	if rec.entries[0].Operation != "restart" {
		t.Errorf("audit operation = %q, want restart", rec.entries[0].Operation)
	}
	if rec.entries[0].Resource != "Application/restaurant-core/staging/web" {
		t.Errorf("audit resource = %q", rec.entries[0].Resource)
	}

	m, _ = step(t, m, done)
	if m.status == "" {
		t.Error("no success status after the mutation completed")
	}
}

func TestLifecycle_CancelDoesNotMutate(t *testing.T) {
	mut := newFakeClient()
	m := NewModel(context.Background(), newFakeClient(), mut)
	m = selectApp(t, m)

	m, _ = step(t, m, keyRunes('S')) // stop
	if m.confirm == nil {
		t.Fatal("S did not arm a confirm prompt")
	}
	m, cmd := step(t, m, keyRunes('n'))
	if m.confirm != nil {
		t.Fatal("n did not cancel the prompt")
	}
	if cmd != nil {
		t.Fatal("cancelling returned a command")
	}
	if len(mut.lifecycle) != 0 {
		t.Fatalf("mutation ran despite cancel: %v", mut.lifecycle)
	}
}

func TestLifecycle_ModalCapturesEveryKey(t *testing.T) {
	mut := newFakeClient()
	m := NewModel(context.Background(), newFakeClient(), mut)
	m = selectApp(t, m)

	m, _ = step(t, m, keyRunes('R'))
	// q must not quit while a confirmation is live.
	m, cmd := step(t, m, keyRunes('q'))
	if cmd != nil {
		t.Fatal("q produced a command during confirm — the modal leaked a key")
	}
	if m.confirm == nil {
		t.Fatal("q dismissed the modal; it must stay until y/n/esc")
	}
	// esc cancels.
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.confirm != nil {
		t.Fatal("esc did not cancel the modal")
	}
	if len(mut.lifecycle) != 0 {
		t.Fatalf("no mutation should have run: %v", mut.lifecycle)
	}
}

func TestLifecycle_APIErrorSurfaced(t *testing.T) {
	mut := newFakeClient()
	mut.lifecycleErr = errors.New("coolify boom")
	m := NewModel(context.Background(), newFakeClient(), mut)
	m = selectApp(t, m)

	m, _ = step(t, m, keyRunes('U')) // start
	m, cmd := step(t, m, keyRunes('y'))
	done, ok := cmd().(mutationDoneMsg)
	if !ok || done.err == nil {
		t.Fatalf("expected a failed mutationDoneMsg, got %#v", done)
	}
	m, _ = step(t, m, done)
	if m.err == nil || !strings.Contains(m.View(), "coolify boom") {
		t.Fatalf("API error not surfaced: %v", m.err)
	}
}

func TestLifecycle_IgnoredOnNonApplication(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(t, m, m.Init()())
	m.tree.cursor = 0 // a project root, not an application leaf
	m, cmd := step(t, m, keyRunes('R'))
	if m.confirm != nil || cmd != nil {
		t.Fatal("lifecycle armed on a non-application node")
	}
}
