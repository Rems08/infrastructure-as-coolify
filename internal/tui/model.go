package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Elm-architecture state of the explore browser. It owns the read-only client,
// the cancelable context shared by every command, the navigation tree, the selected
// resource's detail, and the accumulated log records.
type Model struct {
	ctx    context.Context
	client explorerClient

	keys keyMap
	help help.Model

	tree     tree
	detail   *detail
	logs     []LogMsg
	showLogs bool

	width, height int
	loading       bool
	err           error
}

// NewModel returns a browser model bound to client and ctx. ctx is the context every API
// command runs under; cancelling it (on quit) unblocks any in-flight request.
func NewModel(ctx context.Context, client explorerClient) Model {
	return Model{
		ctx:     ctx,
		client:  client,
		keys:    defaultKeys(),
		help:    help.New(),
		loading: true,
	}
}

// Init kicks off the initial remote resolution.
func (m Model) Init() tea.Cmd {
	return resolveCmd(m.ctx, m.client)
}
