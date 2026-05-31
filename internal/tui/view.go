package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	kindStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	revealedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	confirmStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	addStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	delStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	updStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

// View renders the whole screen: a header, the tree/detail split (or the log pane), and a
// help bar.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("iac-coolify explore"))
	b.WriteByte('\n')

	switch {
	case m.onboarding:
		b.WriteString(m.renderOnboarding())
	case m.showReport:
		b.WriteString(m.renderReport())
	case m.loading:
		b.WriteString(dimStyle.Render("resolving live Coolify state…"))
	case m.showDrift && m.drift != nil:
		b.WriteString(renderDrift(m.drift))
	case m.showLogs:
		b.WriteString(m.renderLogs())
	default:
		b.WriteString(m.renderBrowser())
	}
	if m.confirm != nil {
		b.WriteByte('\n')
		b.WriteString(confirmStyle.Render(m.confirm.prompt))
	}
	if m.status != "" {
		b.WriteByte('\n')
		b.WriteString(statusStyle.Render(m.status))
	}
	if m.err != nil {
		b.WriteByte('\n')
		b.WriteString(errStyle.Render("error: " + m.err.Error()))
	}
	b.WriteByte('\n')
	b.WriteString(m.help.View(m.keys))
	return b.String()
}

// renderBrowser draws the tree on the left and the selected resource's detail on the right.
func (m Model) renderBrowser() string {
	left := m.renderTree()
	right := m.renderDetail()
	return lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
}

func (m Model) renderTree() string {
	rows := m.tree.visible()
	if len(rows) == 0 {
		return dimStyle.Render("(no resources)")
	}
	var b strings.Builder
	for i, row := range rows {
		indent := strings.Repeat("  ", row.depth)
		label := row.node.label + "  " + kindStyle.Render(row.node.kind)
		if i == m.tree.cursor {
			b.WriteString(indent + cursorStyle.Render(treeMarker(row.node)+" "+label))
		} else {
			b.WriteString(indent + treeMarker(row.node) + " " + label)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func treeMarker(n *treeNode) string {
	if n.isLeaf() {
		return "•"
	}
	if n.expanded {
		return "▾"
	}
	return "▸"
}

func (m Model) renderDetail() string {
	if m.detail == nil {
		return dimStyle.Render("select a resource (↵)")
	}
	d := m.detail
	var b strings.Builder
	b.WriteString(titleStyle.Render(d.kind + "  " + d.title))
	b.WriteByte('\n')
	for _, f := range d.fields {
		fmt.Fprintf(&b, "%-16s %s\n", f.label, f.value)
	}
	if d.hasEnvs() {
		q := m.activeFilter()
		rows := filterEnvRows(d.envs, q)
		header := "─ env vars ─"
		if q != "" {
			header += fmt.Sprintf("  filter: %s (%d/%d)", q, len(rows), len(d.envs))
		}
		b.WriteString(dimStyle.Render(header))
		b.WriteByte('\n')
		for _, e := range rows {
			fmt.Fprintf(&b, "%-32s %s%s\n", e.keyLabel(), d.renderEnvValue(e.value), conflictMark(e))
		}
		if d.revealed {
			b.WriteString(revealedStyle.Render("⚠ values revealed (r to hide)"))
		} else {
			b.WriteString(dimStyle.Render("(r to reveal)"))
		}
	}
	if sec := renderDesiredSection(d); sec != "" {
		b.WriteString(sec)
		b.WriteByte('\n')
	}
	if sec := m.renderRemoteOnlySection(); sec != "" {
		b.WriteString(sec)
		b.WriteByte('\n')
	}
	if m.editing != nil {
		fmt.Fprintf(&b, "\nedit %s: %s\n", m.editing.name, m.editing.input.View())
		if m.editing.secret {
			b.WriteString(dimStyle.Render("keep a ${env:NAME} or ${sops:path} reference"))
		}
	}
	if m.filtering != nil {
		fmt.Fprintf(&b, "\nfilter env: %s\n", m.filtering.input.View())
		b.WriteString(dimStyle.Render("↵ apply · esc cancel"))
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderDesiredSection renders an application's desired env-var section: a header with the
// desired↔remote comparison summary, a note when no desired config matched, or one row per env
// var. Each row shows the desired value (plain masked until revealed, secret shown only by its
// source declaration), the joined remote value (masked) and a presence status — tracked when
// the name also exists remotely, only-local when it does not. It returns "" when there is no
// section.
func renderDesiredSection(d *detail) string {
	if d.desiredNote == "" && !d.hasDesiredEnvs() {
		return ""
	}
	var b strings.Builder
	b.WriteString(dimStyle.Render(desiredHeader(d)))
	b.WriteByte('\n')
	if d.remoteEnvErr {
		b.WriteString(dimStyle.Render("remote env unavailable — showing desired only"))
		b.WriteByte('\n')
	}
	if d.desiredNote != "" {
		b.WriteString(dimStyle.Render(d.desiredNote))
		return strings.TrimRight(b.String(), "\n")
	}
	dirty := false
	for i, e := range d.desiredEnvs {
		cursor := "  "
		if i == d.envCursor {
			cursor = cursorStyle.Render("> ")
		}
		marker := ""
		if e.modified {
			marker = updStyle.Render(" *")
			dirty = true
		}
		lock := ""
		if e.secret {
			lock = "🔒 "
		}
		if d.remoteEnvErr {
			fmt.Fprintf(&b, "%s%-22s %s%s%s\n", cursor, e.name, lock, d.renderDesiredValue(e), marker)
			continue
		}
		remote, ok := d.remoteValue(e.name)
		remoteCol, status := "−", dimStyle.Render("only-local")
		if ok {
			remoteCol, status = d.renderEnvValue(remote), statusStyle.Render("tracked")
		}
		fmt.Fprintf(&b, "%s%-20s %s%-22s %-22s %s%s\n", cursor, e.name, lock, d.renderDesiredValue(e), remoteCol, status, marker)
	}
	if dirty {
		b.WriteString(updStyle.Render("● unsaved changes (s save · d discard)"))
		b.WriteByte('\n')
	}
	b.WriteString(dimStyle.Render("(e edit)"))
	if d.hasMaskableValues() {
		if d.revealed {
			b.WriteString(revealedStyle.Render("  ⚠ values revealed (r to hide)"))
		} else {
			b.WriteString(dimStyle.Render("  (r to reveal)"))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// desiredHeader labels the desired section, appending the desired↔remote presence summary when
// a remote listing is available.
func desiredHeader(d *detail) string {
	if d.remoteEnvErr {
		return "─ env vars (desired) ─"
	}
	tracked, onlyLocal, onlyRemote := d.envComparison()
	return fmt.Sprintf("─ env vars (desired) ─  %d tracked · %d only-local · %d only-remote",
		tracked, onlyLocal, onlyRemote)
}

// renderRemoteOnlySection lists the live env vars with no desired counterpart: present on the
// server, absent from the YAML. The rows are read-only (outside the edit cursor) — adopting one
// into the desired config is out of scope here. The active filter narrows the list by key and a
// fixed window scrolls it, so a long list (dozens of uncaptured vars) stays readable. It
// returns "" when every remote var is tracked.
func (m Model) renderRemoteOnlySection() string {
	d := m.detail
	all := d.onlyRemoteEnvs()
	if len(all) == 0 {
		return ""
	}
	q := m.activeFilter()
	rows := filterEnvRows(all, q)

	var b strings.Builder
	header := "─ env vars (only on remote) ─"
	if q != "" {
		header += fmt.Sprintf("  filter: %s (%d/%d)", q, len(rows), len(all))
	}
	b.WriteString(dimStyle.Render(header))
	b.WriteByte('\n')
	if len(rows) == 0 {
		b.WriteString(dimStyle.Render("  (no key matches the filter)"))
		return strings.TrimRight(b.String(), "\n")
	}

	start := d.envScroll
	if start > len(rows) {
		start = len(rows)
	}
	end := start + envWindow
	if end > len(rows) {
		end = len(rows)
	}
	if start > 0 {
		fmt.Fprintf(&b, "%s\n", dimStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
	}
	for _, e := range rows[start:end] {
		fmt.Fprintf(&b, "  %-32s %-22s %s%s\n", e.keyLabel(), d.renderEnvValue(e.value), addStyle.Render("only-remote"), conflictMark(e))
	}
	if end < len(rows) {
		fmt.Fprintf(&b, "%s\n", dimStyle.Render(fmt.Sprintf("  ↓ %d more", len(rows)-end)))
	}
	return strings.TrimRight(b.String(), "\n")
}

// conflictMark flags a row whose collapsed (key, scope) duplicates disagreed on the value. It
// names the inconsistency without printing either value, so it stays leak-safe even revealed.
func conflictMark(e envRow) string {
	if e.conflict {
		return "  " + updStyle.Render("⚠ conflicting values")
	}
	return ""
}

// renderOnboarding draws the no-manifests menu, or a progress line while a sync runs.
func (m Model) renderOnboarding() string {
	var b strings.Builder
	b.WriteString(dimStyle.Render("no local manifests found in " + m.syncDir))
	b.WriteString("\n\n")
	if m.syncing {
		b.WriteString(statusStyle.Render("syncing… importing live resources into YAML"))
		return b.String()
	}
	for _, line := range []struct{ key, rest string }{
		{"[S]", "ync this instance to local YAML"},
		{"[I]", "nit an empty scaffold"},
		{"[B]", "rowse without local config"},
		{"[Q]", "uit"},
	} {
		fmt.Fprintf(&b, "  %s%s\n", cursorStyle.Render(line.key), line.rest)
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderReport shows the last import's outcome (counts and the keys to populate), reusing the
// importer's text rendering. It never prints a secret value.
func (m Model) renderReport() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("sync report"))
	b.WriteByte('\n')
	b.WriteString(strings.TrimRight(m.report, "\n"))
	b.WriteByte('\n')
	b.WriteString(dimStyle.Render("(esc to browse)"))
	return b.String()
}

func (m Model) renderLogs() string {
	if len(m.logs) == 0 {
		return dimStyle.Render("(no logs yet) — esc/L to return")
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render("logs"))
	b.WriteByte('\n')
	for _, l := range m.logs {
		line := fmt.Sprintf("%s %-4s %s", l.Time.Format("15:04:05"), l.Level.String(), l.Message)
		if l.Attrs != "" {
			line += "  " + dimStyle.Render(l.Attrs)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}
