package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func dbDoc(name, engine string) string {
	return fmt.Sprintf(`api_version: iac-coolify/v1
kind: Database
metadata:
  name: %s
  project: beenaire
  environment: staging
spec:
  engine: %s
  image: %s:latest
  destination:
    server: localhost
    network: coolify
`, name, engine, engine)
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDatabases_AllEngines(t *testing.T) {
	engines := []string{"postgresql", "mysql", "mariadb", "mongodb", "redis", "keydb", "dragonfly", "clickhouse"}
	dir := t.TempDir()
	for _, e := range engines {
		writeFile(t, dir, "db-"+e+".yaml", dbDoc("db-"+e, e))
	}
	dbs, err := LoadDatabases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(dbs) != len(engines) {
		t.Fatalf("LoadDatabases = %d databases, want %d", len(dbs), len(engines))
	}
	seen := make(map[string]bool, len(dbs))
	for _, db := range dbs {
		seen[db.Spec.Engine] = true
	}
	for _, e := range engines {
		if !seen[e] {
			t.Errorf("engine %q not loaded", e)
		}
	}
}

func TestLoadDatabases_RejectsUnknownEngine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "bad.yaml", dbDoc("bad", "cockroachdb"))
	if _, err := LoadDatabases(dir); err == nil {
		t.Fatal("want error for unknown engine")
	}
}

func TestLoadDatabases_RejectsMissingDestination(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "nodest.yaml", `api_version: iac-coolify/v1
kind: Database
metadata:
  name: nodest
  project: beenaire
  environment: staging
spec:
  engine: postgresql
  image: postgres:18-alpine
`)
	if _, err := LoadDatabases(dir); err == nil {
		t.Fatal("want error for missing destination")
	}
}

func TestLoadDatabases_Idempotent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pg.yaml", dbDoc("pg", "postgresql"))
	first, err := LoadDatabases(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadDatabases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].Metadata.Name != second[0].Metadata.Name {
		t.Errorf("repeated load diverged: %+v vs %+v", first, second)
	}
}

func fullProject() string { return filepath.Join("..", "..", "examples", "full-project") }

func TestValidateFullProjectExample(t *testing.T) {
	rep, err := Validate(fullProject(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("full-project example must validate clean, issues: %+v", rep.Issues)
	}
	if len(rep.Projects) != 1 || rep.Projects[0] != "beenaire" {
		t.Errorf("Projects = %v, want [beenaire]", rep.Projects)
	}
	if len(rep.Environments) != 1 || rep.Environments[0] != "staging" {
		t.Errorf("Environments = %v, want [staging]", rep.Environments)
	}
	if len(rep.Apps) != 1 || rep.Apps[0] != "api" {
		t.Errorf("Apps = %v, want [api]", rep.Apps)
	}
}

func TestLoadProjectsAndEnvironments(t *testing.T) {
	projects, err := LoadProjects(fullProject())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 || projects[0].Metadata.Name != "beenaire" {
		t.Errorf("LoadProjects = %+v, want one beenaire project", projects)
	}

	envs, err := LoadEnvironments(fullProject())
	if err != nil {
		t.Fatal(err)
	}
	if len(envs) != 1 || envs[0].Metadata.Name != "staging" || envs[0].Metadata.Project != "beenaire" {
		t.Errorf("LoadEnvironments = %+v, want one staging/beenaire environment", envs)
	}

	apps, err := LoadApplications(fullProject())
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].Metadata.Name != "api" {
		t.Errorf("LoadApplications = %+v, want one api application", apps)
	}
}

func TestLoadApplicationFilesKeepsPath(t *testing.T) {
	files, err := LoadApplicationFiles(fullProject())
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("LoadApplicationFiles = %d files, want 1", len(files))
	}
	f := files[0]
	if f.Application.Metadata.Name != "api" {
		t.Errorf("application name = %q, want api", f.Application.Metadata.Name)
	}
	if filepath.Base(f.Path) != "api.yaml" {
		t.Errorf("path base = %q, want api.yaml", filepath.Base(f.Path))
	}
	// LoadApplications must stay functionally identical to the path-aware primitive.
	apps, err := LoadApplications(fullProject())
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != len(files) || apps[0].Metadata.Name != f.Application.Metadata.Name {
		t.Errorf("LoadApplications drifted from LoadApplicationFiles: %+v vs %+v", apps, files)
	}
}

func TestValidateInvalidProjectName(t *testing.T) {
	rep, err := Validate(filepath.Join("testdata", "bad-project.yaml"), false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() {
		t.Fatal("a project with an invalid name must produce an issue")
	}
}

func TestValidateInvalidEnvironment(t *testing.T) {
	rep, err := Validate(filepath.Join("testdata", "bad-environment.yaml"), false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() {
		t.Fatal("an environment missing its project must produce an issue")
	}
}
