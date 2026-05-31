package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// envWindow is how many only-remote rows are shown at once; the rest scroll into view. It is a
// fixed window rather than a height-derived one to keep the split layout simple and the scroll
// math testable.
const envWindow = 15

// filterState is the open env-key filter input. While it is set every key press is routed to
// the textinput (mirroring editState), so typing a query can neither quit nor trigger an
// action mid-typing.
type filterState struct {
	input textinput.Model
}

// activeFilter is the query currently narrowing the env lists: the live input value while the
// filter is being typed, otherwise the applied filter. It is trimmed so trailing spaces never
// hide every row.
func (m Model) activeFilter() string {
	if m.filtering != nil {
		return strings.TrimSpace(m.filtering.input.Value())
	}
	return m.filter
}

// filterEnvRows keeps the rows whose key contains q (case-insensitive). An empty q keeps every
// row. The match is on keys only — values stay masked and are never compared, so the filter
// cannot surface a hidden value.
func filterEnvRows(rows []envRow, q string) []envRow {
	if q == "" {
		return rows
	}
	ql := strings.ToLower(q)
	out := make([]envRow, 0, len(rows))
	for _, e := range rows {
		if strings.Contains(strings.ToLower(e.key), ql) {
			out = append(out, e)
		}
	}
	return out
}

// startFilter opens the env-key filter input, pre-filled with the applied query. It is a no-op
// unless the open detail has env rows to filter (a service table or an application's remote
// env vars).
func (m Model) startFilter() (tea.Model, tea.Cmd) {
	if m.detail == nil || (!m.detail.hasEnvs() && len(m.detail.remoteEnvs) == 0) {
		return m, nil
	}
	ti := textinput.New()
	ti.SetValue(m.filter)
	ti.CursorEnd()
	ti.Focus()
	m.filtering = &filterState{input: ti}
	return m, textinput.Blink
}

// handleFilter consumes a key while the filter input is open. Enter applies the query and keeps
// it active; esc closes the input without changing the applied filter; every other key is
// forwarded to the textinput so it stays modal.
func (m Model) handleFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter = strings.TrimSpace(m.filtering.input.Value())
		m.filtering = nil
		m.clampEnvScroll()
		return m, nil
	case "esc":
		m.filtering = nil
		return m, nil
	default:
		var cmd tea.Cmd
		m.filtering.input, cmd = m.filtering.input.Update(msg)
		m.clampEnvScroll()
		return m, cmd
	}
}

// maxEnvScroll is the largest valid scroll offset for the only-remote list under the active
// filter: zero when the (filtered) list fits the window.
func (d detail) maxEnvScroll(filter string) int {
	n := len(filterEnvRows(d.onlyRemoteEnvs(), filter))
	if n <= envWindow {
		return 0
	}
	return n - envWindow
}

// clampEnvScroll keeps the only-remote scroll offset within bounds after the filter changes, so
// narrowing a list never leaves the view scrolled past its end.
func (m *Model) clampEnvScroll() {
	if m.detail == nil {
		return
	}
	if max := m.detail.maxEnvScroll(m.activeFilter()); m.detail.envScroll > max {
		m.detail.envScroll = max
	}
	if m.detail.envScroll < 0 {
		m.detail.envScroll = 0
	}
}
