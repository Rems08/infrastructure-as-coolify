package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func fullStack() string { return filepath.Join("..", "..", "examples", "full-stack") }

func TestLoadServicesReadsComposeContent(t *testing.T) {
	t.Setenv("GRAFANA_ADMIN_PASSWORD", "from-env")
	services, err := LoadServices(fullStack())
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 {
		t.Fatalf("LoadServices = %d, want 1", len(services))
	}
	ls := services[0]
	if ls.Service.Metadata.Name != "observability-stack" {
		t.Errorf("name = %q, want observability-stack", ls.Service.Metadata.Name)
	}
	if !strings.Contains(ls.ComposeRaw, "grafana/grafana") {
		t.Errorf("compose content not read from the neighbouring file: %q", ls.ComposeRaw)
	}
}

func TestValidateFullStackExample(t *testing.T) {
	t.Setenv("GRAFANA_ADMIN_PASSWORD", "from-env")
	rep, err := Validate(fullStack(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK() {
		t.Fatalf("full-stack example must validate clean, issues: %+v", rep.Issues)
	}
	if len(rep.Services) != 1 || rep.Services[0] != "observability-stack" {
		t.Errorf("Services = %v, want [observability-stack]", rep.Services)
	}
}

func TestValidatePathTraversalAttackRejected(t *testing.T) {
	attack := filepath.Join("..", "..", "examples", "invalid", "services", "path-traversal-attack.yaml")
	rep, err := Validate(attack, false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK() {
		t.Fatal("a service whose compose path escapes the tree must produce an issue")
	}
	if !strings.Contains(rep.Issues[0].Message, "must not escape") {
		t.Errorf("issue = %q, want 'must not escape'", rep.Issues[0].Message)
	}
}

func TestLoadServicesRejectsTraversal(t *testing.T) {
	attackDir := filepath.Join("..", "..", "examples", "invalid", "services")
	if _, err := LoadServices(attackDir); err == nil || !strings.Contains(err.Error(), "must not escape") {
		t.Fatalf("LoadServices = %v, want a traversal rejection", err)
	}
}
