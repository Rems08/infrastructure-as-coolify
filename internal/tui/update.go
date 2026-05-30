package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
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
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Logs):
		m.showLogs = !m.showLogs
		return m, nil
	case key.Matches(msg, m.keys.Reveal):
		if m.detail != nil && m.detail.hasEnvs() {
			m.detail.revealed = !m.detail.revealed
		}
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.tree.up()
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.tree.down()
		return m, nil
	case key.Matches(msg, m.keys.Back):
		m.tree.collapse()
		return m, nil
	case key.Matches(msg, m.keys.Open):
		if leaf := m.tree.toggle(); leaf != nil {
			m.detail = ptr(loadingDetail(leaf))
			return m, loadDetailCmd(m.ctx, m.client, leaf)
		}
		return m, nil
	}
	return m, nil
}

func ptr(d detail) *detail { return &d }
