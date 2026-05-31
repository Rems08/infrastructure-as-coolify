package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// exploreModelFor mirrors the explore command's decision: an empty directory starts on the
// onboarding menu, a populated one browses directly.
func exploreModelFor(t *testing.T, dir string, fc *fakeClient) Model {
	t.Helper()
	opts := []Option{WithConfigPath(dir)}
	has, err := config.HasManifests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		opts = append(opts, WithOnboarding(dir, "https://coolify.example"))
	}
	m := NewModel(context.Background(), fc, fc, opts...)
	m.loading = false
	return m
}

// syncableClient serves one complete application and one database, both in an enumerated
// environment, so a sync writes real manifests the desired index can reload.
func syncableClient() *fakeClient {
	fc := newFakeClient()
	fc.srvRes = map[string][]coolify.ServerResource{
		"s1": {
			{UUID: "a1", Name: "web", Type: "application"},
			{UUID: "db1", Name: "pg", Type: "standalone-postgresql"},
		},
	}
	fc.app = coolify.Application{
		UUID: "a1", Name: "web", BuildPack: "dockerimage",
		DockerRegistryImageName: "registry/web", DockerRegistryImageTag: "v1",
		PortsExposes: "8000", EnvironmentID: 10,
	}
	fc.db = coolify.Database{
		UUID: "db1", Name: "pg", DatabaseType: "standalone-postgresql",
		Image: "postgres:18-alpine", PostgresPassword: secrets.NewRemote("live-pwd"), EnvironmentID: 10,
	}
	fc.appEnvs = []coolify.ServiceEnvVar{{Key: "NODE_ENV", Value: "prod"}}
	return fc
}

func TestOnboarding_EmptyDirShowsMenu(t *testing.T) {
	m := exploreModelFor(t, t.TempDir(), newFakeClient())
	if !m.onboarding {
		t.Fatal("empty directory must start on the onboarding menu")
	}
	view := m.View()
	for _, want := range []string{"[S]", "[I]", "[B]", "[Q]"} {
		if !strings.Contains(view, want) {
			t.Errorf("menu missing %q:\n%s", want, view)
		}
	}
}

func TestOnboarding_PopulatedDirBrowsesDirectly(t *testing.T) {
	m := exploreModelFor(t, desiredPath(), newFakeClient())
	if m.onboarding {
		t.Fatal("a directory with manifests must browse directly, not onboard")
	}
	if strings.Contains(m.View(), "[S]") {
		t.Errorf("browse view must not show the onboarding menu:\n%s", m.View())
	}
}

func TestOnboarding_BrowseLeavesMenu(t *testing.T) {
	m := exploreModelFor(t, t.TempDir(), newFakeClient())
	m, _ = step(t, m, keyRunes('b'))
	if m.onboarding {
		t.Error("B must leave the onboarding menu for the browser")
	}
}

func TestOnboarding_QuitFromMenu(t *testing.T) {
	m := exploreModelFor(t, t.TempDir(), newFakeClient())
	if _, cmd := step(t, m, keyRunes('q')); cmd == nil {
		t.Error("Q from the menu must quit")
	}
	if _, cmd := step(t, m, escKey()); cmd == nil {
		t.Error("esc from the menu must quit")
	}
}

func TestOnboarding_SyncRunsImporterAndLoadsResult(t *testing.T) {
	dir := t.TempDir()
	m := exploreModelFor(t, dir, syncableClient())

	m, cmd := step(t, m, keyRunes('s'))
	if !m.syncing {
		t.Fatal("S must mark a sync in flight")
	}
	done, ok := cmd().(syncDoneMsg)
	if !ok || done.err != nil {
		t.Fatalf("sync command = %+v, want a successful syncDoneMsg", done)
	}

	m, cmd = step(t, m, done)
	if m.onboarding || m.syncing {
		t.Error("a finished sync must leave the menu")
	}
	if !m.showReport {
		t.Error("a finished sync must show its report")
	}
	if m.configPath != dir {
		t.Errorf("configPath = %q, want the synced dir %q", m.configPath, dir)
	}
	for _, rel := range []string{"coolify.yaml", "environments/staging/applications/web.yaml", "environments/staging/databases/pg.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("sync did not write %s: %v", rel, err)
		}
	}
	if !strings.Contains(m.View(), "imported 1 application") {
		t.Errorf("report view missing the import summary:\n%s", m.View())
	}

	// The reload command repopulates the desired index from the freshly written manifests.
	reload, ok := cmd().(desiredLoadedMsg)
	if !ok || reload.err != nil {
		t.Fatalf("reload = %+v, want a successful desiredLoadedMsg", reload)
	}
	m, _ = step(t, m, reload)
	if len(m.desired) != 1 {
		t.Errorf("desired index = %d entries after sync, want 1", len(m.desired))
	}
}

func TestOnboarding_SyncConflictConfirmsThenForces(t *testing.T) {
	dir := t.TempDir()
	collision := filepath.Join(dir, "environments/staging/applications/web.yaml")
	if err := os.MkdirAll(filepath.Dir(collision), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(collision, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := exploreModelFor(t, dir, syncableClient())

	m, cmd := step(t, m, keyRunes('s'))
	done := cmd().(syncDoneMsg)
	m, _ = step(t, m, done)
	if m.confirm == nil || !strings.Contains(m.confirm.prompt, "overwrite") {
		t.Fatalf("a conflict must arm an overwrite confirmation, got %+v", m.confirm)
	}

	// y runs the armed force re-import; n/esc would cancel.
	m, cmd = step(t, m, keyRunes('y'))
	if m.confirm != nil {
		t.Error("y must dismiss the confirmation")
	}
	forced := cmd().(syncDoneMsg)
	if forced.err != nil {
		t.Fatalf("forced re-import failed: %v", forced.err)
	}
	if got, _ := os.ReadFile(collision); string(got) == "stale\n" {
		t.Error("the forced re-import must overwrite the stale manifest")
	}
}

func TestOnboarding_SyncCancelKeepsMenu(t *testing.T) {
	dir := t.TempDir()
	collision := filepath.Join(dir, "environments/staging/applications/web.yaml")
	if err := os.MkdirAll(filepath.Dir(collision), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(collision, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := exploreModelFor(t, dir, syncableClient())
	m, cmd := step(t, m, keyRunes('s'))
	m, _ = step(t, m, cmd().(syncDoneMsg))

	m, _ = step(t, m, keyRunes('n'))
	if m.confirm != nil {
		t.Error("n must dismiss the confirmation")
	}
	if !m.onboarding {
		t.Error("a cancelled overwrite must stay on the menu")
	}
	if got, _ := os.ReadFile(collision); string(got) != "stale\n" {
		t.Error("a cancelled overwrite must not touch the manifest")
	}
}

func TestOnboarding_InitWritesRootScaffold(t *testing.T) {
	dir := t.TempDir()
	m := exploreModelFor(t, dir, newFakeClient())

	m, cmd := step(t, m, keyRunes('i'))
	done, ok := cmd().(initDoneMsg)
	if !ok || done.err != nil || !done.wrote {
		t.Fatalf("init command = %+v, want a successful write", done)
	}
	m, _ = step(t, m, done)
	if m.onboarding {
		t.Error("init must leave the menu for the browser")
	}
	data, err := os.ReadFile(filepath.Join(dir, "coolify.yaml"))
	if err != nil {
		t.Fatalf("init did not write coolify.yaml: %v", err)
	}
	if !strings.Contains(string(data), "https://coolify.example") {
		t.Errorf("scaffold missing the resolved api_url:\n%s", data)
	}
}

func TestOnboarding_InitNoOpWhenRootPresent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "coolify.yaml"), []byte("pre-existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := exploreModelFor(t, dir, newFakeClient())

	m, cmd := step(t, m, keyRunes('i'))
	done := cmd().(initDoneMsg)
	if done.wrote {
		t.Error("init must not overwrite an existing root manifest")
	}
	step(t, m, done)
	if got, _ := os.ReadFile(filepath.Join(dir, "coolify.yaml")); string(got) != "pre-existing\n" {
		t.Error("init overwrote an existing coolify.yaml")
	}
}
