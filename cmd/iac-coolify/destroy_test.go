package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestDestroyCommand_CIRefusesWithoutAutoApprove(t *testing.T) {
	clearCoolifyEnv(t)
	t.Setenv("CI", "true")
	_, err := runCmd(t, "destroy", minimalDir())
	if err == nil {
		t.Fatal("destroy in CI without --auto-approve must error")
	}
	if !strings.Contains(err.Error(), "auto-approve") {
		t.Errorf("error = %v, want it to mention --auto-approve", err)
	}
}

func TestDestroyCommand_DryRunOfflineListsResources(t *testing.T) {
	clearCoolifyEnv(t)
	t.Setenv("CI", "true")
	out, err := runCmd(t, "destroy", minimalDir(), "--auto-approve", "--dry-run", "--output=json")
	if err != nil {
		t.Fatalf("destroy dry-run: %v (out: %s)", err, out)
	}
	var got struct {
		DryRun     bool     `json:"dry_run"`
		ToDestroy  int      `json:"to_destroy"`
		Operations []string `json:"operations"`
	}
	if jErr := json.Unmarshal([]byte(out), &got); jErr != nil {
		t.Fatalf("parse json: %v\n%s", jErr, out)
	}
	if !got.DryRun {
		t.Error("dry_run = false, want true")
	}
	if got.ToDestroy < 1 {
		t.Errorf("to_destroy = %d, want >= 1 (offline dry-run assumes resources present)", got.ToDestroy)
	}
	if indexOfContaining(got.Operations, "delete Application") < 0 {
		t.Errorf("operations = %v, want a delete Application op", got.Operations)
	}
}

// TestDestroyCommand_UnresolvedDestinationEnv proves destroy never needs ${env:} values:
// the destination is irrelevant to a delete (resources are resolved by name), so a manifest
// whose destination.server references an unset variable still tears down cleanly. This is
// the first half of the only supported server-move choreography (destroy, then re-apply).
func TestDestroyCommand_UnresolvedDestinationEnv(t *testing.T) {
	clearCoolifyEnv(t)
	srv, calls := destinationMux(t)
	t.Setenv("COOLIFY_API_TOKEN", "tok")

	manifest := strings.Replace(movedAppManifest, "server: hetzner-1", `server: "${env:UNSET_DESTROY_SERVER}"`, 1)
	out, err := runCmd(t, "destroy", writeManifestDir(t, manifest),
		"--env", "staging", "--auto-approve", "--output=json",
		"--coolify-url", srv.URL, "--openapi-dir", openapiDir(),
		"--audit-log", filepath.Join(t.TempDir(), "audit.log"))
	if err != nil {
		t.Fatalf("destroy with an unresolved destination env: %v\n%s", err, out)
	}
	if indexOfContaining(*calls, "DELETE /api/v1/applications/u-web") < 0 {
		t.Errorf("expected the application DELETE call, got %v", *calls)
	}
}
