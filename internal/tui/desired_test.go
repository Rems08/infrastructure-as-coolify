package tui

import (
	"context"
	"path/filepath"
	"testing"
)

// desiredPath points at the testdata Application that carries env vars (one plain, one
// secret). WEB_DB_URL must be set before loading because an ${env:} secret resolves at
// decode time.
func desiredPath() string { return filepath.Join("testdata", "desired") }

func TestLoadDesiredCmd_BuildsIndexByEnvName(t *testing.T) {
	t.Setenv("WEB_DB_URL", "postgres://example")

	msg, ok := loadDesiredCmd(desiredPath())().(desiredLoadedMsg)
	if !ok {
		t.Fatalf("loadDesiredCmd returned %T, want desiredLoadedMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	m := NewModel(context.Background(), newFakeClient(), newFakeClient())
	m.desired = msg.index

	if f, ok := m.desiredFor("staging", "web"); !ok {
		t.Fatal("desiredFor(staging, web) not found")
	} else if filepath.Base(f.Path) != "web.yaml" {
		t.Errorf("matched file = %q, want web.yaml", f.Path)
	}
	if _, ok := m.desiredFor("production", "web"); ok {
		t.Error("desiredFor matched the wrong environment")
	}
	if _, ok := m.desiredFor("staging", "absent"); ok {
		t.Error("desiredFor matched an absent name")
	}
}

func TestLoadDesiredCmd_EmptyPathYieldsEmptyIndex(t *testing.T) {
	msg, ok := loadDesiredCmd("")().(desiredLoadedMsg)
	if !ok {
		t.Fatalf("loadDesiredCmd returned %T, want desiredLoadedMsg", msg)
	}
	if msg.err != nil {
		t.Fatalf("empty path must not error: %v", msg.err)
	}
	if len(msg.index) != 0 {
		t.Errorf("index = %d entries, want 0", len(msg.index))
	}
}

// TestUpdate_ResolveTriggersDesiredLoad checks that completing the initial resolve dispatches
// the desired-index load and that the resulting message populates the model.
func TestUpdate_ResolveTriggersDesiredLoad(t *testing.T) {
	t.Setenv("WEB_DB_URL", "postgres://example")

	m := NewModel(context.Background(), newFakeClient(), newFakeClient(), WithConfigPath(desiredPath()))
	m, cmd := step(t, m, m.Init()()) // resolvedMsg → returns loadDesiredCmd
	if cmd == nil {
		t.Fatal("resolve did not trigger the desired-index load")
	}
	m, _ = step(t, m, cmd()) // desiredLoadedMsg
	if _, ok := m.desiredFor("staging", "web"); !ok {
		t.Error("desired index not populated after resolve")
	}
}
