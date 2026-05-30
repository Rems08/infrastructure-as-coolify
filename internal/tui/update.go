package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// Update advances the model in response to a message. All remote work is dispatched as a
// tea.Cmd (never run inline), so the loop never blocks on I/O.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.Width = msg.Width
		return m, nil
	case resolvedMsg:
		m.tree = tree{roots: buildTree(msg.m)}
		m.loading = false
		m.err = nil
		return m, nil
	case appDetailMsg:
		m.detail = ptr(applicationDetail(msg.app))
		return m, nil
	case dbDetailMsg:
		m.detail = ptr(databaseDetail(msg.db))
		return m, nil
	case svcDetailMsg:
		m.detail = ptr(serviceDetail(msg.name, msg.envs))
		return m, nil
	case LogMsg:
		m.logs = append(m.logs, msg)
		return m, nil
	case driftMsg:
		m.drift = &driftView{title: msg.title, changes: msg.changes, note: msg.note}
		m.showDrift = true
		m.showLogs = false
		return m, nil
	case mutationDoneMsg:
		if msg.err != nil {
			m.err = msg.err
			m.status = ""
		} else {
			m.err = nil
			m.status = fmt.Sprintf("%s requested for %s", msg.action, msg.name)
		}
		return m, nil
	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleKey applies a key press. Navigation mutates the tree in place; opening a leaf
// returns a command that fetches its detail.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A live confirmation captures every key so an accidental press can neither escape it
	// (e.g. q quitting mid-prompt) nor trigger another action.
	if m.confirm != nil {
		return m.handleConfirm(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Restart):
		return m.startLifecycle(actionRestart)
	case key.Matches(msg, m.keys.Stop):
		return m.startLifecycle(actionStop)
	case key.Matches(msg, m.keys.Start):
		return m.startLifecycle(actionStart)
	case key.Matches(msg, m.keys.Drift):
		return m.toggleDrift()
	case key.Matches(msg, m.keys.Logs):
		m.showLogs = !m.showLogs
		return m, nil
	case key.Matches(msg, m.keys.Reveal):
		if m.detail != nil && m.detail.hasEnvs() {
			m.detail.revealed = !m.detail.revealed
		}
		return m, nil
	}
	return m.handleNav(msg)
}

// handleNav applies the tree-navigation keys (up/down/back/open). Navigation mutates the
// tree in place; opening a leaf returns a command that fetches its detail.
func (m Model) handleNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.tree.up()
	case key.Matches(msg, m.keys.Down):
		m.tree.down()
	case key.Matches(msg, m.keys.Back):
		m.tree.collapse()
	case key.Matches(msg, m.keys.Open):
		if leaf := m.tree.toggle(); leaf != nil {
			m.detail = ptr(loadingDetail(leaf))
			return m, loadDetailCmd(m.ctx, m.client, leaf)
		}
	}
	return m, nil
}

// toggleDrift closes an open drift pane, or computes drift for the selected application.
// On any non-application node it is a no-op.
func (m Model) toggleDrift() (tea.Model, tea.Cmd) {
	if m.showDrift {
		m.showDrift = false
		return m, nil
	}
	n := m.tree.selected()
	if n == nil || !n.isLeaf() || n.kind != resource.KindApplication {
		return m, nil
	}
	return m, driftCmd(m.ctx, m.client, m.configPath, n.key.Name, n.uuid)
}

// startLifecycle arms a confirmation prompt for a lifecycle action on the selected
// application. Lifecycle actions apply to applications only; on any other node it is a
// no-op. The mutation is not run here — it is deferred to the command in confirmState.
func (m Model) startLifecycle(action string) (tea.Model, tea.Cmd) {
	n := m.tree.selected()
	if n == nil || !n.isLeaf() || n.kind != resource.KindApplication {
		return m, nil
	}
	m.confirm = &confirmState{
		prompt:    fmt.Sprintf("%s application %q? [y/N]", action, n.label),
		onConfirm: lifecycleCmd(m.ctx, m.mutator, m.auditor, action, n.key, n.uuid),
	}
	return m, nil
}

// handleConfirm consumes a key press while a confirmation is active. Only y runs the armed
// command; n or esc cancels; every other key is swallowed so the prompt stays modal.
func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		cmd := m.confirm.onConfirm
		m.confirm = nil
		return m, cmd
	case "n", "N", "esc":
		m.confirm = nil
		return m, nil
	default:
		return m, nil
	}
}

func ptr(d detail) *detail { return &d }
