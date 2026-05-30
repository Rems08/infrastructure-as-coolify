package tui

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	return NewModel(context.Background(), newFakeClient(), newFakeClient())
}

func TestNewModel_WiresExplorerAndMutator(t *testing.T) {
	explorer := newFakeClient()
	mutator := newFakeClient()
	rec := &fakeRecorder{}
	m := NewModel(context.Background(), explorer, mutator,
		WithConfigPath("examples/beenaire"),
		WithAuditor(rec),
	)
	if m.client == nil || m.mutator == nil {
		t.Fatal("NewModel left explorer or mutator unwired")
	}
	if m.configPath != "examples/beenaire" {
		t.Errorf("configPath = %q, want examples/beenaire", m.configPath)
	}
	if m.auditor == nil {
		t.Error("WithAuditor did not wire the auditor")
	}
	// A nil auditor must be ignored, not stored.
	if m2 := NewModel(context.Background(), explorer, mutator, WithAuditor(nil)); m2.auditor != nil {
		t.Error("WithAuditor(nil) must leave the auditor unset")
	}
}

func keyRunes(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// step runs Update and returns the concrete Model, failing if it is not one.
func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return got, cmd
}

func TestUpdate_ResolveBuildsTree(t *testing.T) {
	m := newTestModel(t)
	cmd := m.Init()
	msg := cmd() // runs resolveCmd synchronously
	resolved, ok := msg.(resolvedMsg)
	if !ok {
		t.Fatalf("init msg = %T, want resolvedMsg", msg)
	}
	m, _ = step(t, m, resolved)
	if m.loading {
		t.Error("still loading after resolved")
	}
	if len(m.tree.visible()) == 0 {
		t.Fatal("tree empty after resolve")
	}
}

func TestUpdate_QuitReturnsQuitCmd(t *testing.T) {
	m := newTestModel(t)
	_, cmd := step(t, m, keyRunes('q'))
	if cmd == nil {
		t.Fatal("quit returned nil cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("quit cmd produced %T, want tea.QuitMsg", cmd())
	}
}

func TestUpdate_LogMsgAccumulates(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(t, m, LogMsg{Time: time.Now(), Level: slog.LevelInfo, Message: "hello"})
	if len(m.logs) != 1 || m.logs[0].Message != "hello" {
		t.Fatalf("logs = %+v, want one hello", m.logs)
	}
	// L toggles the log pane.
	m, _ = step(t, m, keyRunes('L'))
	if !m.showLogs {
		t.Fatal("L did not open log pane")
	}
	m, _ = step(t, m, keyRunes('L'))
	if m.showLogs {
		t.Fatal("L did not close log pane")
	}
}

func TestUpdate_ErrMsgShownNotCrash(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(t, m, errMsg{errors.New("api down")})
	if m.err == nil || !strings.Contains(m.View(), "api down") {
		t.Fatalf("error not surfaced in view: err=%v", m.err)
	}
}

func TestLoadDetailCmd_ContainerClearsLoading(t *testing.T) {
	// A container kind has no detail endpoint; the command must clear the placeholder rather
	// than return nil (which would leave the pane stuck on "(loading…)").
	node := &treeNode{label: "staging", kind: resource.KindEnvironment}
	msg := loadDetailCmd(context.Background(), newFakeClient(), node)()
	if _, ok := msg.(detailClearedMsg); !ok {
		t.Fatalf("loadDetailCmd on a container = %T, want detailClearedMsg", msg)
	}
}

func TestUpdate_LoadingDetailClearedOnContainerAndError(t *testing.T) {
	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{"container", detailClearedMsg{}},
		{"error", errMsg{errors.New("api down")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.detail = ptr(loadingDetail(&treeNode{label: "staging", kind: resource.KindEnvironment}))
			m, _ = step(t, m, tc.msg)
			if m.detail != nil {
				t.Errorf("loading detail not cleared by %s: %+v", tc.name, m.detail)
			}
		})
	}
}

func TestUpdate_RevealOnlyAffectsServiceDetail(t *testing.T) {
	m := newTestModel(t)
	// Reveal with no detail is a safe no-op.
	m, _ = step(t, m, keyRunes('r'))

	// Application detail has no env table: reveal must not flip anything visible.
	m.detail = ptr(applicationDetail(newFakeClient().app))
	m, _ = step(t, m, keyRunes('r'))
	if m.detail.revealed {
		t.Error("reveal flipped on a detail with no env table")
	}
}

// TestUpdate_IntegrationSmoke drives a full session through the message loop on a fake
// client: resolve, expand to a service, open it, reveal its values, open the log pane and
// quit — asserting the model state at each step without a real terminal.
func TestUpdate_IntegrationSmoke(t *testing.T) {
	m := newTestModel(t)

	m, _ = step(t, m, m.Init()()) // resolvedMsg
	if len(m.tree.visible()) != 2 {
		t.Fatalf("post-resolve visible = %d, want 2", len(m.tree.visible()))
	}

	m, _ = step(t, m, keyRunes('j'))                  // not needed but exercises down at top
	m.tree.cursor = 0                                 // back to project
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand project
	m, _ = step(t, m, keyRunes('j'))                  // staging
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand staging
	m, _ = step(t, m, keyRunes('j'))                  // kafka (service, sorted first)

	// Open the service: returns a detail-load command.
	var cmd tea.Cmd
	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("opening a leaf returned no command")
	}
	m, _ = step(t, m, cmd()) // svcDetailMsg
	if m.detail == nil || !m.detail.hasEnvs() {
		t.Fatal("service detail not loaded with env table")
	}

	// Values masked until revealed.
	view := m.View()
	if !strings.Contains(view, mask) {
		t.Fatal("service env values not masked by default")
	}
	if strings.Contains(view, "s3cr3t-staging-pwd") {
		t.Fatal("secret value shown before reveal")
	}
	m, _ = step(t, m, keyRunes('r'))
	if !strings.Contains(m.View(), "s3cr3t-staging-pwd") {
		t.Fatal("value not revealed after r")
	}

	_, cmd = step(t, m, keyRunes('q'))
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("session did not quit")
	}
}

func TestResolveCmd_ErrorBecomesErrMsg(t *testing.T) {
	fake := newFakeClient()
	fake.listProjectsErr = errors.New("connection refused")
	msg := resolveCmd(context.Background(), fake)()
	em, ok := msg.(errMsg)
	if !ok {
		t.Fatalf("msg = %T, want errMsg", msg)
	}
	if !strings.Contains(em.err.Error(), "connection refused") {
		t.Errorf("error = %v", em.err)
	}
}

func TestUpdate_OpenApplicationAndDatabaseDetail(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(t, m, m.Init()())

	// Drill into the application leaf and fetch it.
	m.tree.cursor = 0
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand project
	m, _ = step(t, m, keyRunes('j'))                  // staging
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand staging
	m, _ = step(t, m, keyRunes('j'))                  // kafka
	m, _ = step(t, m, keyRunes('j'))                  // web (application)

	var cmd tea.Cmd
	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = step(t, m, cmd())
	if m.detail == nil || m.detail.hasEnvs() {
		t.Fatal("application detail should have struct fields, no env table")
	}
	if !strings.Contains(m.View(), "running:healthy") {
		t.Errorf("application status not rendered: %q", m.View())
	}

	// Drill into the unscoped databases group and open the database leaf.
	m, _ = step(t, m, keyRunes('j'))                  // databases group
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // expand group
	m, _ = step(t, m, keyRunes('j'))                  // redis
	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = step(t, m, cmd()) // dbDetailMsg
	if m.detail == nil || m.detail.hasEnvs() {
		t.Fatal("database detail should have struct fields, no env table")
	}
	if !strings.Contains(m.View(), "standalone-redis") {
		t.Errorf("database type not rendered: %q", m.View())
	}
}
