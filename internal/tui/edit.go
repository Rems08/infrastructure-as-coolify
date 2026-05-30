package tui

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// editState is an active env-var edit. While it is set, every key press is routed to the
// textinput (mirroring confirmState) so a stray key can neither quit nor trigger a save
// mid-typing. target and name identify the env var being edited; secret marks a value_secret
// line, whose input must stay a ${env:}/${sops:} reference so a secret can never be turned
// into a literal.
type editState struct {
	input  textinput.Model
	target appKey
	name   string
	secret bool
}

// stagedEnv is one pending env-var edit, not yet written. value is the new plain value or, for
// a secret, the new source declaration (${env:}/${sops:}); secret selects which EnvVarEntry
// field a save patches.
type stagedEnv struct {
	value  string
	secret bool
}

// savedMsg reports the outcome of a write-back to the update loop.
type savedMsg struct {
	target appKey
	path   string
	err    error
}

// validateEdit enforces the anti-leak rule on a confirmed edit. A secret line must remain a
// ${env:}/${sops:} reference (secrets.NewReference rejects a literal); a plain value must be
// non-empty so a save never drops the field on reload.
func validateEdit(secret bool, val string) error {
	if secret {
		if _, err := secrets.NewReference(val); err != nil {
			return err
		}
		return nil
	}
	if val == "" {
		return fmt.Errorf("value cannot be empty")
	}
	return nil
}

// stageEdit records a validated edit for an application's env var. m.staged is initialised at
// construction, so a value receiver mutates the shared maps in place.
func (m Model) stageEdit(target appKey, name, val string, secret bool) {
	if m.staged[target] == nil {
		m.staged[target] = map[string]stagedEnv{}
	}
	m.staged[target][name] = stagedEnv{value: val, secret: secret}
}

// hasPendingEdits reports whether any application carries unsaved edits. The quit guard uses
// it to decide whether to confirm before exiting.
func (m Model) hasPendingEdits() bool {
	for _, edits := range m.staged {
		if len(edits) > 0 {
			return true
		}
	}
	return false
}

// refreshDesired rebuilds an application detail's desired env rows from the matched config
// overlaid with its staged edits, so modified rows show their new value and a marker. It is a
// no-op on a non-application detail or one with no matched config.
func (m Model) refreshDesired(d *detail) {
	if d == nil || d.kind != resource.KindApplication {
		return
	}
	f, ok := m.desiredFor(d.env, d.name)
	if !ok {
		return
	}
	d.desiredEnvs = desiredEnvRowsWithEdits(f.Application.Spec.EnvVars, m.staged[appKey{env: d.env, name: d.name}])
	if d.envCursor >= len(d.desiredEnvs) {
		d.envCursor = 0
	}
}

// patchApplication returns a copy of app with staged edits applied by env-var name. A plain
// edit sets Value; a secret edit sets ValueSecret from its source declaration (never a
// resolved value, which the browser never holds) — exactly one field is set, matching
// EnvVarEntry's invariant. An edit naming an absent var is ignored. It errors only if a secret
// edit is not a valid reference, which staging already rejects, so it is a belt-and-braces
// guard before the write.
func patchApplication(app resource.Application, edits map[string]stagedEnv) (resource.Application, error) {
	vars := make([]resource.EnvVarEntry, len(app.Spec.EnvVars))
	copy(vars, app.Spec.EnvVars)
	for i := range vars {
		e, ok := edits[vars[i].Name]
		if !ok {
			continue
		}
		if e.secret {
			sec, err := secrets.NewReference(e.value)
			if err != nil {
				return resource.Application{}, err
			}
			vars[i] = resource.EnvVarEntry{Name: vars[i].Name, ValueSecret: sec}
			continue
		}
		vars[i] = resource.EnvVarEntry{Name: vars[i].Name, Value: e.value}
	}
	out := app
	out.Spec.EnvVars = vars
	return out, nil
}

// saveCmd writes the patched manifest back to disk off the update loop and traces the write.
// The mutation never runs inline: it is wrapped in the returned tea.Cmd.
func saveCmd(ctx context.Context, aud recorder, path string, app resource.Application, target appKey) tea.Cmd {
	return func() tea.Msg {
		err := config.WriteApplication(path, app)
		traceSave(ctx, aud, path, target, err)
		return savedMsg{target: target, path: path, err: err}
	}
}

// traceSave emits a structured record for every write-back: an slog record (surfaced in the
// log pane) and, when an auditor is wired, an append-only audit entry. Neither carries a
// value; the audit entry records only the operation and the resource identity.
func traceSave(ctx context.Context, aud recorder, path string, target appKey, err error) {
	if err != nil {
		slog.ErrorContext(ctx, "env write-back failed", "path", path, "error", err)
		return
	}
	slog.InfoContext(ctx, "env write-back", "application", target.name, "environment", target.env, "path", path)
	if aud == nil {
		return
	}
	entry := apply.AuditEntry{
		Operation: "write-back",
		Resource:  "Application/" + target.env + "/" + target.name,
	}
	if recErr := aud.Record(entry); recErr != nil {
		slog.WarnContext(ctx, "audit record failed", "error", recErr)
	}
}
