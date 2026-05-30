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

// detail is the right-hand pane: the fields and (for a service) environment variables of
// the selected resource. revealed toggles the env-value mask; it is the only place a value
// is shown, and only after an explicit keypress.
type detail struct {
	kind     string
	title    string
	fields   []field
	envs     []envRow
	revealed bool
}

// hasEnvs reports whether the detail carries an environment-variable table (services only),
// which is what the reveal toggle acts on.
func (d detail) hasEnvs() bool { return len(d.envs) > 0 }

// renderEnvValue returns what the view prints for an env value: the mask unless the user
// has revealed values.
func (d detail) renderEnvValue(v string) string {
	if d.revealed {
		return v
	}
	return mask
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
