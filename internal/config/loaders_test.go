package config

import (
	"path/filepath"
	"testing"
)

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
