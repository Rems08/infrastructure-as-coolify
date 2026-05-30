package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
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
		return m, loadDesiredCmd(m.configPath)
	case desiredLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.desired = msg.index
		return m, nil
	case appDetailMsg:
		d := applicationDetail(msg.app)
		m.attachDesired(&d, msg.env, msg.name)
		m.detail = &d
		return m, nil
	case dbDetailMsg:
		m.detail = ptr(databaseDetail(msg.db))
		return m, nil
	case svcDetailMsg:
		m.detail = ptr(serviceDetail(msg.name, msg.envs))
		return m, nil
	case savedMsg:
		return m.applySaved(msg)
	case LogMsg:
		m.logs = append(m.logs, msg)
		return m, nil
	case driftMsg:
		m.drift = &driftView{title: msg.title, changes: msg.changes, note: msg.note}
		m.showDrift = true
		m.showLogs = false
		return m, nil
	case mutationDoneMsg:
		return m.applyMutationDone(msg)
	case errMsg:
		m.err = msg.err
		m.loading = false
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// applySaved records the outcome of a write-back. A failure surfaces an error and keeps the
// staging so the user can retry; a success purges the saved application's edits and refreshes
// the display.
func (m Model) applySaved(msg savedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.status = ""
		return m, nil
	}
	delete(m.staged, msg.target)
	m.err = nil
	m.status = "saved " + msg.path
	m.refreshDesired(m.detail)
	return m, nil
}

// applyMutationDone records the outcome of a lifecycle action in the status or error line.
func (m Model) applyMutationDone(msg mutationDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.status = ""
		return m, nil
	}
	m.err = nil
	m.status = fmt.Sprintf("%s requested for %s", msg.action, msg.name)
	return m, nil
}

// handleKey applies a key press. Navigation mutates the tree in place; opening a leaf
// returns a command that fetches its detail.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A live confirmation or edit captures every key so an accidental press can neither escape
	// it (e.g. q quitting mid-prompt) nor trigger another action.
	if m.confirm != nil {
		return m.handleConfirm(msg)
	}
	if m.editing != nil {
		return m.handleEdit(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m.handleQuit()
	case key.Matches(msg, m.keys.Edit):
		return m.startEdit()
	case key.Matches(msg, m.keys.Save):
		return m.startSave()
	case key.Matches(msg, m.keys.Discard):
		return m.discardEdits()
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
		return m.toggleReveal()
	}
	return m.handleNav(msg)
}

// handleQuit quits immediately when clean; with unsaved edits it arms a confirmation instead,
// so a stray q cannot discard pending write-backs.
func (m Model) handleQuit() (tea.Model, tea.Cmd) {
	if m.hasPendingEdits() {
		m.confirm = &confirmState{prompt: "uncommitted changes — quit anyway? [y/N]", onConfirm: tea.Quit}
		return m, nil
	}
	return m, tea.Quit
}

// toggleReveal flips the value mask when the detail has maskable values; otherwise a no-op.
func (m Model) toggleReveal() (tea.Model, tea.Cmd) {
	if m.detail != nil && m.detail.hasMaskableValues() {
		m.detail.revealed = !m.detail.revealed
	}
	return m, nil
}

// handleNav applies the tree-navigation keys (up/down/back/open). When an application detail
// with desired env rows is open, focus moves into that list instead, so up/down select the
// row to edit and back returns focus to the tree.
func (m Model) handleNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.detail != nil && m.detail.hasDesiredEnvs() {
		return m.handleDesiredNav(msg)
	}
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

// handleDesiredNav moves the cursor over an application's desired env rows, or returns focus
// to the tree (back). Up/down clamp at the ends; open is a no-op (e edits the cursored row).
func (m Model) handleDesiredNav(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.detail.envCursor > 0 {
			m.detail.envCursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.detail.envCursor < len(m.detail.desiredEnvs)-1 {
			m.detail.envCursor++
		}
	case key.Matches(msg, m.keys.Back):
		m.detail = nil
	}
	return m, nil
}

// startEdit opens the textinput on the cursored desired env row, pre-filled with the plain
// value or, for a secret, its source declaration. It is a no-op unless an application detail
// with desired env rows is open.
func (m Model) startEdit() (tea.Model, tea.Cmd) {
	if m.detail == nil || !m.detail.hasDesiredEnvs() {
		return m, nil
	}
	row := m.detail.desiredEnvs[m.detail.envCursor]
	ti := textinput.New()
	ti.SetValue(row.display)
	ti.CursorEnd()
	ti.Focus()
	m.editing = &editState{
		input:  ti,
		target: appKey{env: m.detail.env, name: m.detail.name},
		name:   row.name,
		secret: row.secret,
	}
	return m, textinput.Blink
}

// handleEdit consumes a key while an edit is active. Enter validates and stages the value
// (keeping the input open on a validation error); esc cancels; every other key is forwarded
// to the textinput so it stays modal.
func (m Model) handleEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.editing.input.Value())
		if err := validateEdit(m.editing.secret, val); err != nil {
			m.err = err
			return m, nil
		}
		m.stageEdit(m.editing.target, m.editing.name, val, m.editing.secret)
		m.editing = nil
		m.err = nil
		m.refreshDesired(m.detail)
		return m, nil
	case "esc":
		m.editing = nil
		m.err = nil
		return m, nil
	default:
		var cmd tea.Cmd
		m.editing.input, cmd = m.editing.input.Update(msg)
		return m, cmd
	}
}

// startSave writes the viewed application's staged edits back to its manifest. It builds the
// patched application inline (pure, no I/O) and defers the disk write to a command, so the
// update loop never blocks. A validation failure surfaces an error and keeps the staging.
func (m Model) startSave() (tea.Model, tea.Cmd) {
	if m.detail == nil || !m.detail.hasDesiredEnvs() {
		return m, nil
	}
	target := appKey{env: m.detail.env, name: m.detail.name}
	edits := m.staged[target]
	if len(edits) == 0 {
		m.status = "no changes to save"
		return m, nil
	}
	f, ok := m.desiredFor(target.env, target.name)
	if !ok {
		return m, nil
	}
	patched, err := patchApplication(f.Application, edits)
	if err != nil {
		m.err = err
		return m, nil
	}
	return m, saveCmd(m.ctx, m.auditor, f.Path, patched, target)
}

// discardEdits drops the viewed application's staged edits and restores the displayed rows to
// the on-disk config. It is a no-op when no application detail with desired rows is open.
func (m Model) discardEdits() (tea.Model, tea.Cmd) {
	if m.detail == nil || !m.detail.hasDesiredEnvs() {
		return m, nil
	}
	target := appKey{env: m.detail.env, name: m.detail.name}
	if len(m.staged[target]) == 0 {
		return m, nil
	}
	delete(m.staged, target)
	m.refreshDesired(m.detail)
	m.status = "discarded edits"
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
