package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// driftView is the right-hand pane showing the difference between the desired config and
// the live resource. note is set instead of changes when drift cannot be computed (no local
// config, or no desired resource of that name) — the view reports it rather than crashing.
type driftView struct {
	title   string
	changes []plan.Change
	note    string
}

// driftMsg carries a computed drift back to the update loop.
type driftMsg struct {
	title   string
	changes []plan.Change
	note    string
}

// driftCmd computes the drift for one application: it loads the desired config, matches the
// remote resource by logical name, and field-diffs the two projections. Matching is by name,
// never by file path, so it needs no desired-file mapping. It is read-only.
func driftCmd(ctx context.Context, client explorerClient, configPath, name, uuid string) tea.Cmd {
	return func() tea.Msg {
		if configPath == "" {
			return driftMsg{title: name, note: "drift unavailable: pass a config path — `explore <path>`"}
		}
		apps, err := config.LoadApplications(configPath)
		if err != nil {
			return errMsg{err}
		}
		desired, ok := findApp(apps, name)
		if !ok {
			return driftMsg{title: name, note: fmt.Sprintf("drift unavailable: no Application %q in local config", name)}
		}
		remote, err := client.GetApplication(ctx, uuid)
		if err != nil {
			return errMsg{err}
		}
		actual := plan.FromRemoteApplication(remote)
		return driftMsg{title: name, changes: plan.Diff(plan.FromApplication(desired), &actual)}
	}
}

func findApp(apps []resource.Application, name string) (resource.Application, bool) {
	for _, a := range apps {
		if a.Metadata.Name == name {
			return a, true
		}
	}
	return resource.Application{}, false
}

func renderDrift(d *driftView) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("drift  " + d.title))
	b.WriteByte('\n')
	if d.note != "" {
		b.WriteString(dimStyle.Render(d.note))
		return b.String()
	}
	if len(d.changes) == 0 {
		b.WriteString(dimStyle.Render("no drift — desired matches live state"))
	} else {
		for _, c := range d.changes {
			b.WriteString(renderChange(c))
			b.WriteByte('\n')
		}
	}
	b.WriteString(dimStyle.Render("read-only — apply via the CLI; in-TUI editing lands later"))
	return strings.TrimRight(b.String(), "\n")
}

func renderChange(c plan.Change) string {
	style := updStyle
	switch c.Op {
	case plan.OpAdd:
		style = addStyle
	case plan.OpDelete:
		style = delStyle
	}
	if c.Sensitive {
		return style.Render(fmt.Sprintf("%s %s 🔒 %s → %s", opGlyph(c.Op), c.Path, redactDisplay(c.Old), redactDisplay(c.New)))
	}
	return style.Render(fmt.Sprintf("%s %s  %s → %s", opGlyph(c.Op), c.Path, c.Old, c.New))
}

func opGlyph(op plan.Op) string {
	switch op {
	case plan.OpAdd:
		return "+"
	case plan.OpDelete:
		return "-"
	default:
		return "~"
	}
}

// redactDisplay shows a secret's source declaration (always safe) but masks anything else,
// so a resolved value can never reach the drift view even if one were ever present. The diff
// engine already keeps secrets source-only; this is a second, view-level guard.
func redactDisplay(s string) string {
	if s == "" || strings.HasPrefix(s, "${") {
		return s
	}
	return mask
}
