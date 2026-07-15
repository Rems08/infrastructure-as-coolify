package importer

import (
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
)

func TestMapEngine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
		ok   bool
	}{
		{name: "postgres", in: "standalone-postgresql", want: "postgresql", ok: true},
		{name: "redis", in: "standalone-redis", want: "redis", ok: true},
		{name: "mongodb", in: "standalone-mongodb", want: "mongodb", ok: true},
		{name: "empty", in: "", want: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapEngine(tt.in)
			if (err == nil) != tt.ok {
				t.Fatalf("mapEngine(%q) err=%v, want ok=%v", tt.in, err, tt.ok)
			}
			if got != tt.want {
				t.Errorf("mapEngine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFirstPort(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "single", in: "8000", want: 8000},
		{name: "multi keeps first", in: "3000,8080", want: 3000},
		{name: "spaces", in: " 5432 , 6000 ", want: 5432},
		{name: "empty", in: "", want: 0},
		{name: "non-numeric", in: "http", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstPort(tt.in); got != tt.want {
				t.Errorf("firstPort(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeEnvName(t *testing.T) {
	tests := []struct{ in, want string }{
		{in: "pg-restaurant-api-staging", want: "PG_RESTAURANT_API_STAGING"},
		{in: "db.prod", want: "DB_PROD"},
		{in: "Already_OK1", want: "ALREADY_OK1"},
	}
	for _, tt := range tests {
		if got := sanitizeEnvName(tt.in); got != tt.want {
			t.Errorf("sanitizeEnvName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMapApplication_DockerimageComplete(t *testing.T) {
	app := coolify.Application{
		Name:                    "api",
		Description:             "core API",
		FQDN:                    "https://api.example.com",
		BuildPack:               "dockerimage",
		DockerRegistryImageName: "registry/api",
		DockerRegistryImageTag:  "v1",
		PortsExposes:            "8000",
		EnvironmentID:           10,
	}
	env := envRef{project: "beenaire", name: "staging"}
	envs := []coolify.ServiceEnvVar{{Key: "NODE_ENV", Value: "production"}}

	mapped, keys := mapApplication(app, "localhost", "coolify", env, envs)
	if vErr := mapped.Validate(); vErr != nil {
		t.Fatalf("dockerimage application must validate: %v", vErr)
	}
	if mapped.Spec.Image == nil || mapped.Spec.Image.Name != "registry/api" || mapped.Spec.Port != 8000 {
		t.Errorf("image/port mapping wrong: %+v port=%d", mapped.Spec.Image, mapped.Spec.Port)
	}
	if mapped.Spec.Description != "core API" {
		t.Errorf("description mapping wrong: %q", mapped.Spec.Description)
	}
	if mapped.Spec.Destination.Server != "localhost" || mapped.Spec.Destination.Network != "coolify" {
		t.Errorf("destination mapping wrong: %+v", mapped.Spec.Destination)
	}
	if len(mapped.Spec.EnvVars) != 1 || mapped.Spec.EnvVars[0].Value != "${env:NODE_ENV}" {
		t.Errorf("env var must become a reference, got %+v", mapped.Spec.EnvVars)
	}
	if mapped.Spec.EnvVars[0].Value == "production" {
		t.Error("env var clear value must never be written")
	}
	if len(keys) != 1 || keys[0] != "NODE_ENV" {
		t.Errorf("referenced keys = %v, want [NODE_ENV]", keys)
	}
}

func TestMapApplication_DedupesEnvKeys(t *testing.T) {
	app := coolify.Application{
		Name: "api", BuildPack: "dockerimage",
		DockerRegistryImageName: "registry/api", DockerRegistryImageTag: "v1", PortsExposes: "8000", EnvironmentID: 10,
	}
	// The live API can return the same key twice (build-time and runtime copies).
	envs := []coolify.ServiceEnvVar{
		{Key: "LOG_LEVEL", Value: "info"},
		{Key: "PORT", Value: "8000"},
		{Key: "LOG_LEVEL", Value: "info"},
	}
	mapped, keys := mapApplication(app, "localhost", "coolify", envRef{project: "p", name: "staging"}, envs)
	if len(mapped.Spec.EnvVars) != 2 {
		t.Errorf("duplicate env keys must collapse to one entry, got %d", len(mapped.Spec.EnvVars))
	}
	if len(keys) != 2 {
		t.Errorf("referenced keys must be de-duplicated, got %v", keys)
	}
}

func TestMapApplication_GitPartial(t *testing.T) {
	app := coolify.Application{
		Name:          "worker",
		BuildPack:     "nixpacks",
		GitBranch:     "main",
		PortsExposes:  "3000",
		EnvironmentID: 10,
	}
	env := envRef{project: "beenaire", name: "staging"}

	mapped, _ := mapApplication(app, "localhost", "coolify", env, nil)
	if mapped.Spec.Source == nil || mapped.Spec.Source.GitRepository != gitRepositoryPlaceholder {
		t.Fatalf("git source must carry the sentinel placeholder, got %+v", mapped.Spec.Source)
	}
	if vErr := mapped.Validate(); vErr == nil {
		t.Error("a git application with a placeholder repository must not validate (it is partial)")
	}
}

func TestMapDatabase(t *testing.T) {
	db := coolify.Database{
		Name:          "pg-api-staging",
		DatabaseType:  "standalone-postgresql",
		Image:         "postgres:18-alpine",
		IsPublic:      false,
		EnvironmentID: 10,
	}
	env := envRef{project: "beenaire", name: "staging"}

	mapped, passwordEnv, err := mapDatabase(db, "localhost", "coolify", env)
	if err != nil {
		t.Fatal(err)
	}
	if mapped.Spec.Engine != "postgresql" {
		t.Errorf("engine = %q, want postgresql", mapped.Spec.Engine)
	}
	if passwordEnv != "PG_API_STAGING_PASSWORD" {
		t.Errorf("password env = %q, want PG_API_STAGING_PASSWORD", passwordEnv)
	}
	if mapped.Spec.Password.Origin() != "${env:PG_API_STAGING_PASSWORD}" {
		t.Errorf("password must be a synthetic reference, got origin %q", mapped.Spec.Password.Origin())
	}
	if vErr := mapped.Validate(); vErr != nil {
		t.Fatalf("database must validate: %v", vErr)
	}
}
