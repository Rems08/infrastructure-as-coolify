package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// loadEnvFile reads a dotenv-style file and sets each variable in the process environment so
// ${env:} references can resolve from a single per-app secret instead of dozens of separate
// CI variables. Format: KEY=VALUE per line, # comments, blank lines, an optional `export `
// prefix, and optional surrounding single or double quotes around the value. The value may
// contain '=' (only the first '=' splits). No variable interpolation and no inline comments
// (a value may legitimately contain '#', e.g. a URL fragment).
//
// An existing environment variable is never overwritten, so a real env var (e.g. a token
// injected by CI) always wins over the file. An empty path is a no-op; a missing or malformed
// file is a loud error. Returns the number of variables set.
func loadEnvFile(path string) (int, error) {
	if path == "" {
		return 0, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("env-file: %w", err)
	}
	defer func() { _ = f.Close() }()

	n, line := 0, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line++
		raw := strings.TrimSpace(sc.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		raw = strings.TrimPrefix(raw, "export ")
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			return n, fmt.Errorf("env-file: line %d is not KEY=VALUE: %q", line, sc.Text())
		}
		key := strings.TrimSpace(raw[:eq])
		val := unquoteEnvValue(strings.TrimSpace(raw[eq+1:]))
		if _, ok := os.LookupEnv(key); ok {
			continue // the real environment wins over the file
		}
		if err := os.Setenv(key, val); err != nil {
			return n, fmt.Errorf("env-file: set %s: %w", key, err)
		}
		n++
	}
	if err := sc.Err(); err != nil {
		return n, fmt.Errorf("env-file: %w", err)
	}
	return n, nil
}

// unquoteEnvValue strips one layer of matching surrounding single or double quotes.
func unquoteEnvValue(s string) string {
	if len(s) >= 2 {
		if c := s[0]; (c == '"' || c == '\'') && s[len(s)-1] == c {
			return s[1 : len(s)-1]
		}
	}
	return s
}
