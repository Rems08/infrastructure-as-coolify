package main

import (
	"os"
	"strings"
	"testing"
)

func TestExploreCommand_RegisteredWithAlias(t *testing.T) {
	root := newRootCmd()
	var explore, tui bool
	for _, c := range root.Commands() {
		if c.Name() == "explore" {
			explore = true
			for _, a := range c.Aliases {
				if a == "tui" {
					tui = true
				}
			}
		}
	}
	if !explore {
		t.Fatal("explore command not registered")
	}
	if !tui {
		t.Error("explore is missing the tui alias")
	}
}

func TestExploreCommand_RefusesNonInteractive(t *testing.T) {
	// Under `go test`, stdout is not a character device, so the interactive guard fires
	// before any terminal is opened.
	out, err := runCmd(t, "explore")
	if err == nil {
		t.Fatal("explore in a non-interactive session: want error")
	}
	if !strings.Contains(err.Error(), "interactive terminal") {
		t.Errorf("error = %v, want interactive-terminal message", err)
	}
	_ = out
}

func TestExploreClient_RefusesWithoutCredentials(t *testing.T) {
	// t.Setenv records the originals for restoration; unset both so buildClient reports
	// offline and exploreClient turns that into a credential error.
	t.Setenv("COOLIFY_API_URL", "")
	t.Setenv("COOLIFY_API_TOKEN", "")
	os.Unsetenv("COOLIFY_API_URL")
	os.Unsetenv("COOLIFY_API_TOKEN")

	_, err := exploreClient("")
	if err == nil {
		t.Fatal("exploreClient with no credentials: want error")
	}
	if !strings.Contains(err.Error(), "credentials") {
		t.Errorf("error = %v, want credentials message", err)
	}
}
