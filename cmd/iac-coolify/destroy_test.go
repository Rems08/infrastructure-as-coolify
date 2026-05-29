package main

import (
	"encoding/json"
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
