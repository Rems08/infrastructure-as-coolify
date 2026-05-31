package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Elm-architecture state of the explore browser. It owns the read-only client,
// the separate mutator client used for lifecycle actions, the cancelable context shared by
// every command, the navigation tree, the selected resource's detail, and the accumulated
// log records.
type Model struct {
	ctx        context.Context
	client     explorerClient
	mutator    mutatorClient
	auditor    recorder     // nil disables persistent audit logging; the slog trace still fires.
	configPath string       // desired-state root for drift; empty disables the drift view.
	desired    desiredIndex // desired Applications keyed by (environment, name); loaded after resolve.

	keys keyMap
	help help.Model

	tree      tree
	detail    *detail
	confirm   *confirmState
	editing   *editState   // active env-var edit; while set, every key routes to the textinput.
	filtering *filterState // open env-key filter input; while set, every key routes to it.
	filter    string       // applied env-key filter; empty shows every row.
	drift     *driftView
	logs      []LogMsg
	showLogs  bool
	showDrift bool

	// onboarding shows the no-manifests menu (sync/init/browse) instead of the tree; syncDir
	// and syncURL are the import target and the URL recorded in the scaffold. syncing marks an
	// import in flight; report holds the last import's rendered outcome and showReport displays
	// it. They are only set when explore starts in a directory with no resource manifests.
	onboarding bool
	syncing    bool
	syncDir    string
	syncURL    string
	report     string
	showReport bool

	// staged holds unsaved env-var edits keyed by application then var name. It is written by
	// s and cleared on save or discard; hasPendingEdits drives the quit guard.
	staged map[appKey]map[string]stagedEnv

	width, height int
	loading       bool
	status        string
	err           error
}

// Option configures a Model at construction.
type Option func(*Model)

// WithConfigPath points the drift view at the desired-state config root. Without it, drift
// is unavailable and the view says so rather than crashing.
func WithConfigPath(path string) Option { return func(m *Model) { m.configPath = path } }

// WithAuditor wires a persistent audit sink for lifecycle actions. A nil auditor is ignored,
// leaving only the slog trace.
func WithAuditor(a recorder) Option {
	return func(m *Model) {
		if a != nil {
			m.auditor = a
		}
	}
}

// NewModel returns a browser model bound to the read-only explorer and the mutator. ctx is
// the context every API command runs under; cancelling it (on quit) unblocks any in-flight
// request. A single *coolify.Client may be passed as both arguments.
func NewModel(ctx context.Context, explorer explorerClient, mutator mutatorClient, opts ...Option) Model {
	m := Model{
		ctx:     ctx,
		client:  explorer,
		mutator: mutator,
		keys:    defaultKeys(),
		help:    help.New(),
		loading: true,
		staged:  map[appKey]map[string]stagedEnv{},
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// Init kicks off the initial remote resolution.
func (m Model) Init() tea.Cmd {
	return resolveCmd(m.ctx, m.client)
}
