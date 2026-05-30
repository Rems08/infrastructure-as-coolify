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
)

// View renders the whole screen: a header, the tree/detail split (or the log pane), and a
// help bar.
func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("iac-coolify explore"))
	b.WriteByte('\n')

	switch {
	case m.loading:
		b.WriteString(dimStyle.Render("resolving live Coolify state…"))
	case m.showLogs:
		b.WriteString(m.renderLogs())
	default:
		b.WriteString(m.renderBrowser())
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
		b.WriteString(dimStyle.Render("─ env vars ─"))
		b.WriteByte('\n')
		for _, e := range d.envs {
			fmt.Fprintf(&b, "%-22s %s\n", e.key, d.renderEnvValue(e.value))
		}
		if d.revealed {
			b.WriteString(revealedStyle.Render("⚠ values revealed (r to hide)"))
		} else {
			b.WriteString(dimStyle.Render("(r to reveal)"))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) renderLogs() string {
	if len(m.logs) == 0 {
		return dimStyle.Render("(no logs yet) — L to return")
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
