package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/config"
)

// appKey identifies a desired Application by its logical coordinates. The desired side is
// matched to a selected remote resource by (environment, name) — never by file path — so it
// needs no filename convention.
type appKey struct {
	env  string
	name string
}

// desiredIndex maps each desired Application to the file it was loaded from, keyed by
// (environment, name). It is built once from the config path and read when an application
// detail is shown.
type desiredIndex map[appKey]config.ApplicationFile

// desiredLoadedMsg carries the built index back to the update loop. An empty config path
// yields an empty index (the desired display is simply unavailable, never a crash); a real
// load failure carries err instead.
type desiredLoadedMsg struct {
	index desiredIndex
	err   error
}

// loadDesiredCmd builds the desired-application index off the update loop. An empty config
// path yields an empty index rather than an error, so browsing without a config path stays
// graceful.
func loadDesiredCmd(configPath string) tea.Cmd {
	return func() tea.Msg {
		if configPath == "" {
			return desiredLoadedMsg{index: desiredIndex{}}
		}
		files, err := config.LoadApplicationFiles(configPath)
		if err != nil {
			return desiredLoadedMsg{err: err}
		}
		idx := make(desiredIndex, len(files))
		for _, f := range files {
			meta := f.Application.Metadata
			idx[appKey{env: meta.Environment, name: meta.Name}] = f
		}
		return desiredLoadedMsg{index: idx}
	}
}

// desiredFor returns the desired Application file matching env and name, if one was indexed.
func (m Model) desiredFor(env, name string) (config.ApplicationFile, bool) {
	f, ok := m.desired[appKey{env: env, name: name}]
	return f, ok
}

// attachDesired fills in an application detail's desired env-var section: the matched
// config's env vars, or a note when no desired Application matches the selection.
func (m Model) attachDesired(d *detail, env, name string) {
	f, ok := m.desiredFor(env, name)
	if !ok {
		d.desiredNote = "no desired config for this application"
		return
	}
	d.desiredEnvs = desiredEnvRows(f.Application.Spec.EnvVars)
}
