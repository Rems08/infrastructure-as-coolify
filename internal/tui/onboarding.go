package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/importer"
)

// rootManifestName is the project root manifest init scaffolds. It carries no resource kind,
// so a directory holding only it still counts as having no manifests.
const rootManifestName = "coolify.yaml"

// WithOnboarding starts the browser on the no-manifests menu, offering to sync the live
// instance into dir or scaffold an empty root there. apiURL is recorded in whatever the menu
// writes. The command passes it only when the target directory holds no resource manifests.
func WithOnboarding(dir, apiURL string) Option {
	return func(m *Model) {
		m.onboarding = true
		m.syncDir = dir
		m.syncURL = apiURL
	}
}

// syncDoneMsg and initDoneMsg report the outcome of the two onboarding actions back to the
// update loop. Neither carries a secret: a sync report lists only counts and the keys to
// populate, and init reports only whether the root was written.
type syncDoneMsg struct {
	report importer.Report
	err    error
}
type initDoneMsg struct {
	wrote bool
	err   error
}

// handleOnboarding routes a key press while the no-manifests menu is shown. Keys are matched
// directly (not through the browse keymap) so the menu's letters never collide with a
// lifecycle binding. While a sync is in flight every key is swallowed.
func (m Model) handleOnboarding(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.syncing {
		return m, nil
	}
	switch strings.ToLower(msg.String()) {
	case "s":
		return m.startSync(false)
	case "i":
		return m.startInit()
	case "b":
		m.onboarding = false
		return m, nil
	case "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

// startSync launches an import of the live instance into the target directory off the update
// loop. force overwrites existing manifests; it is set only after the overwrite confirmation.
func (m Model) startSync(force bool) (tea.Model, tea.Cmd) {
	m.syncing = true
	m.err = nil
	return m, syncCmd(m.ctx, m.client, m.syncDir, m.syncURL, force)
}

// syncCmd runs the importer off the update loop and reports the outcome. The browser's
// read-only client satisfies importer.Client, so no separate wiring is needed; the import
// writes ${env:} references, never resolved secret values.
func syncCmd(ctx context.Context, client importer.Client, dir, apiURL string, force bool) tea.Cmd {
	return func() tea.Msg {
		rep, err := importer.Run(ctx, client, importer.Options{
			Dir:            dir,
			DefaultNetwork: "coolify",
			Force:          force,
			APIURL:         apiURL,
		})
		return syncDoneMsg{report: rep, err: err}
	}
}

// startInit scaffolds an empty root manifest in the target directory off the update loop.
func (m Model) startInit() (tea.Model, tea.Cmd) {
	m.err = nil
	return m, initCmd(m.syncDir, m.syncURL)
}

func initCmd(dir, apiURL string) tea.Cmd {
	return func() tea.Msg {
		wrote, err := importer.ScaffoldRoot(dir, apiURL)
		return initDoneMsg{wrote: wrote, err: err}
	}
}

// applySyncDone records an import's outcome. A pre-existing-manifests conflict arms an
// overwrite confirmation that re-runs the import with force; any other error stays on the menu
// with the error shown. On success it shows the report, points drift at the synced directory,
// reloads the desired index, and leaves the menu for the browser.
func (m Model) applySyncDone(msg syncDoneMsg) (tea.Model, tea.Cmd) {
	m.syncing = false
	if msg.err != nil {
		if isConflict(msg.err) {
			m.confirm = &confirmState{
				prompt:    "existing manifests found — overwrite? [y/N]",
				onConfirm: syncCmd(m.ctx, m.client, m.syncDir, m.syncURL, true),
			}
			return m, nil
		}
		m.err = msg.err
		return m, nil
	}
	m.report = msg.report.RenderText()
	m.showReport = true
	m.onboarding = false
	m.configPath = m.syncDir
	m.status = syncSummary(msg.report)
	return m, loadDesiredCmd(m.configPath)
}

// applyInitDone records the scaffold outcome and leaves the menu for the browser, pointing
// drift at the directory so a later sync or hand-written manifest is picked up.
func (m Model) applyInitDone(msg initDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m, nil
	}
	m.onboarding = false
	m.configPath = m.syncDir
	if msg.wrote {
		m.status = "wrote " + rootManifestName + " — sync or add resources to populate it"
	} else {
		m.status = rootManifestName + " already present"
	}
	return m, loadDesiredCmd(m.configPath)
}

func syncSummary(r importer.Report) string {
	return fmt.Sprintf("synced %d application(s), %d database(s) — esc to browse",
		len(r.Applications), len(r.Databases))
}

// isConflict reports whether err is the importer's refusal to overwrite existing manifests.
func isConflict(err error) bool { return errors.Is(err, importer.ErrManifestsExist) }
