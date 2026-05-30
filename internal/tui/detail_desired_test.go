package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// modelWithDesired returns a model whose desired index is built from the testdata config.
func modelWithDesired(t *testing.T) Model {
	t.Helper()
	t.Setenv("WEB_DB_URL", "postgres://example")
	msg, ok := loadDesiredCmd(desiredPath())().(desiredLoadedMsg)
	if !ok || msg.err != nil {
		t.Fatalf("desired load failed: %+v", msg)
	}
	m := NewModel(context.Background(), newFakeClient(), newFakeClient(), WithConfigPath(desiredPath()))
	m.desired = msg.index
	m.loading = false // skip the resolve screen; these tests render the detail pane directly.
	return m
}

func TestApplicationDetail_DesiredEnvShownPlainAndSecretOrigin(t *testing.T) {
	m := modelWithDesired(t)
	m, _ = step(t, m, appDetailMsg{app: coolify.Application{Name: "web"}, env: "staging", name: "web"})

	if m.detail == nil || !m.detail.hasDesiredEnvs() {
		t.Fatal("desired env section not attached")
	}
	view := m.View()
	// The secret is shown by its source declaration, never its resolved value.
	if !strings.Contains(view, "${env:WEB_DB_URL}") {
		t.Errorf("secret origin not shown:\n%s", view)
	}
	if strings.Contains(view, "postgres://example") {
		t.Fatalf("resolved secret value leaked into the view:\n%s", view)
	}
	// The plain value is masked until revealed.
	if strings.Contains(view, "production") {
		t.Errorf("plain value shown before reveal:\n%s", view)
	}
	if !strings.Contains(view, mask) {
		t.Errorf("plain value not masked:\n%s", view)
	}
	m, _ = step(t, m, keyRunes('r'))
	revealed := m.View()
	if !strings.Contains(revealed, "production") {
		t.Errorf("plain value not revealed after r:\n%s", revealed)
	}
	// Revealing never turns a secret into its value.
	if strings.Contains(revealed, "postgres://example") {
		t.Fatalf("reveal leaked the secret value:\n%s", revealed)
	}
}

func TestApplicationDetail_NoDesiredMatchShowsNote(t *testing.T) {
	m := modelWithDesired(t)
	m, _ = step(t, m, appDetailMsg{app: coolify.Application{Name: "ghost"}, env: "staging", name: "ghost"})

	if m.detail == nil || m.detail.hasDesiredEnvs() {
		t.Fatal("an unmatched application must carry no desired env rows")
	}
	if !strings.Contains(m.View(), "no desired config for this application") {
		t.Errorf("missing no-desired-config note:\n%s", m.View())
	}
}

// TestDesiredEnvRows_SecretCarriesOriginNotValue checks the projection at the unit level: a
// secret entry yields its origin and is flagged secret; Reveal is never involved.
func TestDesiredEnvRows_SecretCarriesOriginNotValue(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "super-secret")
	sec, err := secrets.NewFromEnv("SECRET_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	rows := desiredEnvRows([]resource.EnvVarEntry{
		{Name: "PLAIN", Value: "v1"},
		{Name: "TOKEN", ValueSecret: sec},
	})
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].secret || rows[0].display != "v1" {
		t.Errorf("plain row = %+v, want {display:v1 secret:false}", rows[0])
	}
	if !rows[1].secret || rows[1].display != "${env:SECRET_TOKEN}" {
		t.Errorf("secret row = %+v, want origin display + secret:true", rows[1])
	}
	if strings.Contains(rows[1].display, "super-secret") {
		t.Fatal("secret value leaked into the display")
	}
}
