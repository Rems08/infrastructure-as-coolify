package tui

import (
	"fmt"
	"strconv"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// mask is shown in place of every environment-variable value until the user reveals them.
// It is a fixed string so it never leaks a value's length.
const mask = "••••••"

// field is one labelled scalar of a resource's struct view (applications and databases).
type field struct {
	label string
	value string
}

// envRow is one environment variable of a service. Value is the plaintext the read path
// already returns (coolify.ServiceEnvVar.Value is a plain string, never an opaque Secret);
// masking it is a view concern, so the row holds the cleartext and the view decides
// whether to show it.
//
// scope is the buildtime/runtime label from the live API ("build", "runtime" or
// "build,runtime"), empty for a desired row or when the API set no scope flag. conflict marks
// a row whose collapsed (key, scope) duplicates carried differing values, so the view can flag
// the inconsistency without exposing either value.
type envRow struct {
	key      string
	value    string
	scope    string
	conflict bool
}

// keyLabel renders the row's key with its scope tag appended ("KEY [build,runtime]"), or the
// bare key when the row has no scope.
func (e envRow) keyLabel() string {
	if e.scope == "" {
		return e.key
	}
	return e.key + " [" + e.scope + "]"
}

// envScope renders a live env var's buildtime/runtime scope as a compact label, or "" when the
// API set neither flag.
func envScope(e coolify.ServiceEnvVar) string {
	switch {
	case e.IsBuildtime && e.IsRuntime:
		return "build,runtime"
	case e.IsBuildtime:
		return "build"
	case e.IsRuntime:
		return "runtime"
	default:
		return ""
	}
}

// desiredEnvRow is one environment variable from the desired YAML config. It is leak-proof
// by construction: a plain value is held verbatim (visible config, maskable like a service
// value), while a secret carries only its source declaration (${env:…}/${sops:…}) — never a
// resolved value, which this view never reads.
type desiredEnvRow struct {
	name     string
	display  string // plain value, or a secret's source declaration
	secret   bool
	modified bool // carries an unsaved staged edit
}

// detail is the right-hand pane: the fields, the live env table (services only), and the
// desired env vars (applications with a matched config) of the selected resource. revealed
// toggles the value mask; it is the only place a plain value is shown, and only after an
// explicit keypress.
type detail struct {
	kind        string
	title       string
	fields      []field
	envs        []envRow
	desiredEnvs []desiredEnvRow
	// remoteEnvs are an application's live env vars, joined to desiredEnvs by name for the
	// desired↔remote comparison. remoteEnvErr marks a failed listing: the comparison is then
	// reported unavailable instead of misreporting every desired var as only-local.
	remoteEnvs   []envRow
	remoteEnvErr bool
	desiredNote  string
	revealed     bool
	// loading marks a placeholder shown while a leaf's detail is fetched, so an error or a
	// kind with no detail endpoint can clear it instead of leaving it stuck.
	loading bool

	// env and name are the logical coordinates of an application detail, used to match the
	// desired config and to target an edit; envCursor selects the desired env row to edit.
	env, name string
	envCursor int
	// envScroll is the first visible row of the only-remote list, advanced once the cursor has
	// run past the editable desired rows so a long list stays navigable.
	envScroll int
}

// hasEnvs reports whether the detail carries a live environment-variable table (services
// only), which is what the reveal toggle acts on.
func (d detail) hasEnvs() bool { return len(d.envs) > 0 }

// hasDesiredEnvs reports whether the detail carries a desired env-var section.
func (d detail) hasDesiredEnvs() bool { return len(d.desiredEnvs) > 0 }

// hasMaskableValues reports whether the reveal toggle has anything to act on: a service's
// live env table, an application's remote env values, or a plain (non-secret) desired value.
// Secret desired rows show only their source declaration, so reveal never affects them.
func (d detail) hasMaskableValues() bool {
	if d.hasEnvs() || len(d.remoteEnvs) > 0 {
		return true
	}
	for _, e := range d.desiredEnvs {
		if !e.secret {
			return true
		}
	}
	return false
}

// remoteValue returns the application's live value for a desired env name, and whether that
// name exists on the remote at all (a desired var absent here is only-local).
func (d detail) remoteValue(name string) (string, bool) {
	for _, e := range d.remoteEnvs {
		if e.key == name {
			return e.value, true
		}
	}
	return "", false
}

// onlyRemoteEnvs returns the live env vars with no desired counterpart: keys present on the
// server but absent from the YAML — what is left to capture as IAC. Order follows the remote
// listing.
func (d detail) onlyRemoteEnvs() []envRow {
	if len(d.remoteEnvs) == 0 {
		return nil
	}
	desired := d.desiredNameSet()
	var out []envRow
	for _, e := range d.remoteEnvs {
		if _, ok := desired[e.key]; !ok {
			out = append(out, e)
		}
	}
	return out
}

func (d detail) desiredNameSet() map[string]struct{} {
	s := make(map[string]struct{}, len(d.desiredEnvs))
	for _, e := range d.desiredEnvs {
		s[e.name] = struct{}{}
	}
	return s
}

// envComparison counts the desired↔remote join by presence (never by value: a desired value is
// often a ${env:…} reference, not a resolved value, so a value diff would be meaningless).
func (d detail) envComparison() (tracked, onlyLocal, onlyRemote int) {
	for _, e := range d.desiredEnvs {
		if _, ok := d.remoteValue(e.name); ok {
			tracked++
		} else {
			onlyLocal++
		}
	}
	// only-remote is counted by unique key: a key present on the remote in both scopes is one
	// uncaptured var, even though it renders as two scoped rows.
	seen := map[string]struct{}{}
	for _, e := range d.onlyRemoteEnvs() {
		if _, ok := seen[e.key]; !ok {
			seen[e.key] = struct{}{}
			onlyRemote++
		}
	}
	return tracked, onlyLocal, onlyRemote
}

// renderEnvValue returns what the view prints for a live env value: the mask unless the user
// has revealed values.
func (d detail) renderEnvValue(v string) string {
	if d.revealed {
		return v
	}
	return mask
}

// renderDesiredValue returns what the view prints for a desired env value: a secret's source
// declaration always (never a value, which is never held), or a plain value masked until
// revealed.
func (d detail) renderDesiredValue(e desiredEnvRow) string {
	if e.secret || d.revealed {
		return e.display
	}
	return mask
}

// desiredEnvRows projects desired env vars into display rows. A secret row carries the
// secret's source declaration (Origin) — Reveal is never called, so a resolved secret value
// never enters the model.
func desiredEnvRows(entries []resource.EnvVarEntry) []desiredEnvRow {
	rows := make([]desiredEnvRow, 0, len(entries))
	for _, e := range entries {
		if !e.ValueSecret.IsZero() {
			rows = append(rows, desiredEnvRow{name: e.Name, display: e.ValueSecret.Origin(), secret: true})
			continue
		}
		rows = append(rows, desiredEnvRow{name: e.Name, display: e.Value})
	}
	return rows
}

// desiredEnvRowsWithEdits projects desired env vars overlaid with their staged edits. A staged
// row shows its new value (the source declaration for a secret) and is flagged modified, so a
// secret's resolved value is still never read.
func desiredEnvRowsWithEdits(entries []resource.EnvVarEntry, edits map[string]stagedEnv) []desiredEnvRow {
	rows := desiredEnvRows(entries)
	for i := range rows {
		if e, ok := edits[rows[i].name]; ok {
			rows[i].display = e.value
			rows[i].secret = e.secret
			rows[i].modified = true
		}
	}
	return rows
}

func applicationDetail(app coolify.Application) detail {
	image := app.DockerRegistryImageName
	if app.DockerRegistryImageTag != "" {
		image += ":" + app.DockerRegistryImageTag
	}
	return detail{
		kind:  resource.KindApplication,
		title: app.Name,
		fields: []field{
			{"uuid", app.UUID},
			{"status", app.Status},
			{"build_pack", app.BuildPack},
			{"fqdn", app.FQDN},
			{"image", image},
			{"git_branch", app.GitBranch},
			{"ports_exposes", app.PortsExposes},
		},
	}
}

func databaseDetail(db coolify.Database) detail {
	return detail{
		kind:  resource.KindDatabase,
		title: db.Name,
		fields: []field{
			{"uuid", db.UUID},
			{"status", db.Status},
			{"database_type", db.DatabaseType},
			{"image", db.Image},
			{"is_public", strconv.FormatBool(db.IsPublic)},
			{"public_port", strconv.Itoa(db.PublicPort)},
			{"enable_ssl", strconv.FormatBool(db.EnableSSL)},
		},
	}
}

func serviceDetail(name string, envs []coolify.ServiceEnvVar) detail {
	return detail{
		kind:  resource.KindService,
		title: name,
		envs:  envRows(envs),
	}
}

// envRows projects live env vars into display rows, collapsing the exact (key, scope)
// duplicates the API returns while keeping genuine scope variants (the same key as a build
// entry and a runtime entry) as distinct rows. When collapsed duplicates disagree on the
// value the kept row is flagged conflict. The wire value is plain text (Secret is never
// populated from a response); masking it is a view concern handled by renderEnvValue.
func envRows(envs []coolify.ServiceEnvVar) []envRow {
	rows := make([]envRow, 0, len(envs))
	seen := make(map[string]int, len(envs))
	for _, e := range envs {
		scope := envScope(e)
		id := e.Key + "\x00" + scope
		if i, ok := seen[id]; ok {
			if rows[i].value != e.Value {
				rows[i].conflict = true
			}
			continue
		}
		seen[id] = len(rows)
		rows = append(rows, envRow{key: e.Key, value: e.Value, scope: scope})
	}
	return rows
}

// placeholder describes a leaf that has been selected but whose detail has not arrived yet.
func loadingDetail(node *treeNode) detail {
	return detail{kind: node.kind, title: fmt.Sprintf("%s (loading…)", node.label), loading: true}
}
