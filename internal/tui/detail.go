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
type envRow struct {
	key   string
	value string
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
	desiredNote string
	revealed    bool

	// env and name are the logical coordinates of an application detail, used to match the
	// desired config and to target an edit; envCursor selects the desired env row to edit.
	env, name string
	envCursor int
}

// hasEnvs reports whether the detail carries a live environment-variable table (services
// only), which is what the reveal toggle acts on.
func (d detail) hasEnvs() bool { return len(d.envs) > 0 }

// hasDesiredEnvs reports whether the detail carries a desired env-var section.
func (d detail) hasDesiredEnvs() bool { return len(d.desiredEnvs) > 0 }

// hasMaskableValues reports whether the reveal toggle has anything to act on: a service's
// live env table, or a plain (non-secret) desired value. Secret desired rows show only their
// source declaration, so reveal never affects them.
func (d detail) hasMaskableValues() bool {
	if d.hasEnvs() {
		return true
	}
	for _, e := range d.desiredEnvs {
		if !e.secret {
			return true
		}
	}
	return false
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
	rows := make([]envRow, 0, len(envs))
	for _, e := range envs {
		rows = append(rows, envRow{key: e.Key, value: e.Value})
	}
	return detail{
		kind:  resource.KindService,
		title: name,
		envs:  rows,
	}
}

// placeholder describes a leaf that has been selected but whose detail has not arrived yet.
func loadingDetail(node *treeNode) detail {
	return detail{kind: node.kind, title: fmt.Sprintf("%s (loading…)", node.label)}
}
