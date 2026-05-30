package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

// webManifest is the desired Application used by the write-back tests: one plain env var and
// one secret sourced from ${env:WEB_DB_URL}.
const webManifest = `api_version: iac-coolify/v1
kind: Application
metadata:
  name: web
  project: restaurant-core
  environment: staging
spec:
  build_pack: dockerimage
  image:
    name: registry.example.com/web
    tag: v2-0-0
  destination:
    server: localhost
    network: coolify
  fqdn: https://web.example.com
  port: 8080
  env_vars:
    - name: NODE_ENV
      value: production
    - name: DATABASE_URL
      value_secret: "${env:WEB_DB_URL}"
`

// openWebDetail returns a model with the desired index loaded and the web application detail
// open, so its desired env rows are focused and editable.
func openWebDetail(t *testing.T) Model {
	t.Helper()
	m := modelWithDesired(t)
	m, _ = step(t, m, appDetailMsg{app: coolify.Application{Name: "web"}, env: "staging", name: "web"})
	if m.detail == nil || !m.detail.hasDesiredEnvs() {
		t.Fatal("web detail not opened with desired env rows")
	}
	return m
}

func TestEdit_OpensPrefilledPlainAndSecret(t *testing.T) {
	m := openWebDetail(t)

	// Cursor starts on the plain row: e pre-fills the literal value.
	m, _ = step(t, m, keyRunes('e'))
	if m.editing == nil {
		t.Fatal("e did not open an edit")
	}
	if m.editing.secret {
		t.Error("plain row opened as a secret edit")
	}
	if got := m.editing.input.Value(); got != "production" {
		t.Errorf("plain edit prefilled %q, want production", got)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	// Move to the secret row: e pre-fills the source declaration, never a value.
	m, _ = step(t, m, keyRunes('j'))
	m, _ = step(t, m, keyRunes('e'))
	if m.editing == nil || !m.editing.secret {
		t.Fatal("secret row did not open a secret edit")
	}
	if got := m.editing.input.Value(); got != "${env:WEB_DB_URL}" {
		t.Errorf("secret edit prefilled %q, want the source declaration", got)
	}
}

func TestEdit_CapturesKeysWhileTyping(t *testing.T) {
	m := openWebDetail(t)
	m, _ = step(t, m, keyRunes('e'))

	// A key that would otherwise quit or act is captured by the textinput instead.
	m, _ = step(t, m, keyRunes('q'))
	if m.editing == nil {
		t.Fatal("q escaped the edit instead of being typed")
	}
	if !strings.HasSuffix(m.editing.input.Value(), "q") {
		t.Errorf("typed key not captured: %q", m.editing.input.Value())
	}
}

func TestEdit_SecretRejectsLiteral(t *testing.T) {
	m := openWebDetail(t)
	m, _ = step(t, m, keyRunes('j')) // secret row
	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("postgres://leak")

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.err == nil {
		t.Fatal("a literal secret value was accepted")
	}
	if m.editing == nil {
		t.Error("edit closed despite the validation error")
	}
	if m.hasPendingEdits() {
		t.Error("a rejected edit must not be staged")
	}
}

func TestEdit_PlainAcceptsLiteralAndStages(t *testing.T) {
	m := openWebDetail(t)
	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("devmode")

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.editing != nil {
		t.Fatal("edit did not close after a valid value")
	}
	if !m.hasPendingEdits() {
		t.Fatal("valid edit was not staged")
	}
	row := m.detail.desiredEnvs[0]
	if !row.modified || row.display != "devmode" {
		t.Errorf("staged row = %+v, want modified devmode", row)
	}
}

func TestStage_AccumulatesAndMarksUnsaved(t *testing.T) {
	m := openWebDetail(t)

	// Stage a plain edit.
	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("devmode")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// Stage a secret edit (still a reference).
	m, _ = step(t, m, keyRunes('j'))
	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("${env:OTHER_DB}")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.staged[appKey{env: "staging", name: "web"}]) != 2 {
		t.Fatalf("expected two staged edits, got %v", m.staged)
	}
	view := m.View()
	if !strings.Contains(view, "unsaved changes") {
		t.Errorf("unsaved-changes indicator missing:\n%s", view)
	}
	if !strings.Contains(view, "${env:OTHER_DB}") {
		t.Errorf("edited secret reference not shown:\n%s", view)
	}
	if strings.Contains(view, "postgres://example") {
		t.Fatalf("a resolved secret value leaked:\n%s", view)
	}
}

// writeTempManifest writes webManifest to a fresh dir and returns the dir, so write-back tests
// never mutate the shared testdata fixture.
func writeTempManifest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "web.yaml"), []byte(webManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// modelEditingTemp loads the desired index from a writable temp manifest and opens the web
// detail, so a save writes to that copy.
func modelEditingTemp(t *testing.T, dir string, rec recorder) Model {
	t.Helper()
	t.Setenv("WEB_DB_URL", "postgres://example")
	msg, ok := loadDesiredCmd(dir)().(desiredLoadedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("desired load failed: %+v", msg)
	}
	m := NewModel(context.Background(), newFakeClient(), newFakeClient(), WithConfigPath(dir), WithAuditor(rec))
	m.desired = msg.index
	m.loading = false
	m, _ = step(t, m, appDetailMsg{app: coolify.Application{Name: "web"}, env: "staging", name: "web"})
	return m
}

func TestSave_WritesPatchedManifestAndAudits(t *testing.T) {
	dir := writeTempManifest(t)
	rec := &fakeRecorder{}
	m := modelEditingTemp(t, dir, rec)

	// Edit the plain value and save.
	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("devmode")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	m, cmd := step(t, m, keyRunes('s'))
	if cmd == nil {
		t.Fatal("save returned no command (write-back must run as a tea.Cmd)")
	}
	m, _ = step(t, m, cmd()) // savedMsg

	out, err := os.ReadFile(filepath.Join(dir, "web.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "devmode") {
		t.Errorf("patched value not written:\n%s", got)
	}
	if strings.Contains(got, "production") {
		t.Errorf("old value still present:\n%s", got)
	}
	// The untouched secret round-trips as its source declaration, never a resolved value.
	if !strings.Contains(got, "${env:WEB_DB_URL}") {
		t.Errorf("secret source declaration not preserved:\n%s", got)
	}
	if strings.Contains(got, "postgres://example") {
		t.Fatalf("resolved secret value written to disk:\n%s", got)
	}
	if m.hasPendingEdits() {
		t.Error("staging not purged after a successful save")
	}
	if len(rec.entries) != 1 || rec.entries[0].Operation != "write-back" {
		t.Fatalf("write-back not audited: %+v", rec.entries)
	}
}

func TestSave_EditedSecretRoundTripsAsOrigin(t *testing.T) {
	dir := writeTempManifest(t)
	m := modelEditingTemp(t, dir, nil)

	m, _ = step(t, m, keyRunes('j')) // secret row
	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("${env:OTHER_DB}")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	m, cmd := step(t, m, keyRunes('s'))
	m, _ = step(t, m, cmd())

	out, _ := os.ReadFile(filepath.Join(dir, "web.yaml"))
	got := string(out)
	if !strings.Contains(got, "${env:OTHER_DB}") {
		t.Errorf("edited secret reference not written:\n%s", got)
	}
	if strings.Contains(got, "postgres://example") {
		t.Fatalf("resolved secret value leaked to disk:\n%s", got)
	}
}

func TestSave_WriteErrorKeepsStaging(t *testing.T) {
	dir := writeTempManifest(t)
	m := modelEditingTemp(t, dir, nil)

	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("devmode")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// Point the indexed file at a non-existent directory so the atomic write fails.
	target := appKey{env: "staging", name: "web"}
	f := m.desired[target]
	f.Path = filepath.Join(dir, "missing", "web.yaml")
	m.desired[target] = f

	m, cmd := step(t, m, keyRunes('s'))
	m, _ = step(t, m, cmd()) // savedMsg with err
	if m.err == nil {
		t.Fatal("a failed write did not surface an error")
	}
	if !m.hasPendingEdits() {
		t.Error("staging dropped after a failed write")
	}
}

func TestDiscard_PurgesWithoutWriting(t *testing.T) {
	dir := writeTempManifest(t)
	m := modelEditingTemp(t, dir, nil)

	m, _ = step(t, m, keyRunes('e'))
	m.editing.input.SetValue("devmode")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.hasPendingEdits() {
		t.Fatal("edit not staged before discard")
	}

	m, _ = step(t, m, keyRunes('d'))
	if m.hasPendingEdits() {
		t.Error("discard did not purge staging")
	}
	if m.detail.desiredEnvs[0].modified {
		t.Error("row still marked modified after discard")
	}
	// Nothing was written: the on-disk value is unchanged.
	out, _ := os.ReadFile(filepath.Join(dir, "web.yaml"))
	if !strings.Contains(string(out), "production") {
		t.Errorf("discard wrote to disk:\n%s", out)
	}
}
