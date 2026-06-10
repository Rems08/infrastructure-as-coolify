package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.env")
	body := "# a comment\n" +
		"\n" +
		"DJANGO_SETTINGS_MODULE=config.settings.staging\n" +
		"export DATABASE_URL=postgres://u:p@h:5432/db?sslmode=require\n" +
		"QUOTED=\"with spaces\"\n" +
		"SINGLE='single'\n" +
		"ALREADY_SET=from-file\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	// A real env var must win over the file.
	t.Setenv("ALREADY_SET", "from-env")
	// Ensure the keys we assert on are clean.
	for _, k := range []string{"DJANGO_SETTINGS_MODULE", "DATABASE_URL", "QUOTED", "SINGLE"} {
		_ = os.Unsetenv(k)
		t.Cleanup(func() { _ = os.Unsetenv(k) }) //nolint:gocritic // best-effort cleanup
	}

	n, err := loadEnvFile(path)
	if err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if n != 4 {
		t.Errorf("set count = %d, want 4 (5 assignments, ALREADY_SET skipped)", n)
	}
	cases := map[string]string{
		"DJANGO_SETTINGS_MODULE": "config.settings.staging",
		"DATABASE_URL":           "postgres://u:p@h:5432/db?sslmode=require", // '=' inside value preserved
		"QUOTED":                 "with spaces",
		"SINGLE":                 "single",
		"ALREADY_SET":            "from-env", // env wins
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoadEnvFile_EmptyPathIsNoop(t *testing.T) {
	if n, err := loadEnvFile(""); err != nil || n != 0 {
		t.Fatalf("empty path: n=%d err=%v, want 0,nil", n, err)
	}
}

func TestLoadEnvFile_MissingAndMalformed(t *testing.T) {
	if _, err := loadEnvFile(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Error("missing file should error")
	}
	bad := filepath.Join(t.TempDir(), "bad.env")
	if err := os.WriteFile(bad, []byte("NOT_AN_ASSIGNMENT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadEnvFile(bad); err == nil {
		t.Error("malformed line should error")
	}
}
