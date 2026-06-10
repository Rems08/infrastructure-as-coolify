package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// matchesEnv reports whether env is in the allow set. An empty allow set means no
// environment filter is active, so every resource matches.
func matchesEnv(allow []string, env string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, e := range allow {
		if e == env {
			return true
		}
	}
	return false
}

// filterByEnv keeps the items whose environment is in the allow set. With no filter it
// returns items unchanged; envOf extracts the environment name a given item belongs to.
func filterByEnv[T any](items []T, allow []string, envOf func(T) string) []T {
	if len(allow) == 0 {
		return items
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if matchesEnv(allow, envOf(it)) {
			out = append(out, it)
		}
	}
	return out
}

// filterByName keeps the items whose logical name equals only. An empty only means no name
// filter is active, so every item is kept. Used to narrow by --target before resolving
// secrets, so a scoped apply only binds the env vars its own resources reference.
func filterByName[T any](items []T, only string, nameOf func(T) string) []T {
	if only == "" {
		return items
	}
	out := make([]T, 0, len(items))
	for _, it := range items {
		if nameOf(it) == only {
			out = append(out, it)
		}
	}
	return out
}

// exit2 prints a user-facing error and requests process exit code 2, the code reserved for
// an invalid selection (a misspelled --env or an empty --target/--env combination) so a typo
// fails loudly instead of silently selecting nothing.
func exit2(cmd *cobra.Command, format string, args ...any) error {
	fmt.Fprintf(cmd.ErrOrStderr(), "iac-coolify: "+format+"\n", args...)
	return exitErr{code: 2}
}

// envScope records which environments the loaded resources declare and how many of them pass
// the combined --target/--env filter, so a command can reject an invalid selection in one
// pass before doing any work.
type envScope struct {
	only      string
	envFilter []string
	present   map[string]struct{}
	selected  int
}

func newEnvScope(only string, envFilter []string) *envScope {
	return &envScope{only: only, envFilter: envFilter, present: map[string]struct{}{}}
}

// add records one environment-scoped resource (an application, service, database, or the
// environment itself), contributing its environment to the universe and to the selection
// count when it passes both filters.
func (s *envScope) add(name, env string) {
	s.present[env] = struct{}{}
	if selected(s.only, name) && matchesEnv(s.envFilter, env) {
		s.selected++
	}
}

// addCrossEnv records a resource that has no environment of its own (a project): it never
// contributes to the environment universe but still counts toward a name selection.
func (s *envScope) addCrossEnv(name string) {
	if selected(s.only, name) {
		s.selected++
	}
}

// validate fails with exit code 2 when a requested environment is declared by no resource,
// or when a combined --target/--env selection matches nothing.
func (s *envScope) validate(cmd *cobra.Command, path string) error {
	for _, e := range s.envFilter {
		if _, ok := s.present[e]; !ok {
			return exit2(cmd, "environment %q matches no resources in %s", e, path)
		}
	}
	if s.only != "" && len(s.envFilter) > 0 && s.selected == 0 {
		return exit2(cmd, "no resources match --target=%s --env=%s in %s", s.only, strings.Join(s.envFilter, ","), path)
	}
	return nil
}
