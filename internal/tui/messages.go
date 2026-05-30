package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// resolvedMsg carries the resolved UUID map back to the update loop after the initial
// remote resolution completes.
type resolvedMsg struct{ m state.Map }

// appDetailMsg, dbDetailMsg and svcDetailMsg carry a fetched resource's detail. A service
// also carries its environment variables (the only resource with a listable env set on the
// read path); applications and databases expose their fields through the struct alone. An
// application carries its (environment, name) coordinates so the update loop can match it to
// the desired config without a second lookup of the selection.
type appDetailMsg struct {
	app       coolify.Application
	env, name string
}
type dbDetailMsg struct{ db coolify.Database }
type svcDetailMsg struct {
	name string
	envs []coolify.ServiceEnvVar
}

// errMsg reports a failed command. It is rendered in a panel rather than crashing the
// program.
type errMsg struct{ err error }

// resolveCmd resolves the live UUID map. The client satisfies state.Resolve's narrower
// interface, so it is passed straight through.
func resolveCmd(ctx context.Context, client explorerClient) tea.Cmd {
	return func() tea.Msg {
		m, err := state.Resolve(ctx, client)
		if err != nil {
			return errMsg{err}
		}
		return resolvedMsg{m}
	}
}

// loadDetailCmd fetches the detail of a leaf resource. The endpoint is chosen from the
// node kind; a service additionally lists its environment variables.
func loadDetailCmd(ctx context.Context, client explorerClient, node *treeNode) tea.Cmd {
	uuid, kind, name, env := node.uuid, node.kind, node.label, node.key.Environment
	return func() tea.Msg {
		switch kind {
		case resource.KindApplication:
			app, err := client.GetApplication(ctx, uuid)
			if err != nil {
				return errMsg{err}
			}
			return appDetailMsg{app: app, env: env, name: name}
		case resource.KindDatabase:
			db, err := client.GetDatabase(ctx, uuid)
			if err != nil {
				return errMsg{err}
			}
			return dbDetailMsg{db}
		case resource.KindService:
			envs, err := client.ListServiceEnvs(ctx, uuid)
			if err != nil {
				return errMsg{err}
			}
			return svcDetailMsg{name: name, envs: envs}
		default:
			return nil
		}
	}
}
