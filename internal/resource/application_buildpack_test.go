package resource_test

import (
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func appWith(spec resource.ApplicationSpec) resource.Application {
	return resource.Application{
		APIVersion: "iac-coolify/v1",
		Kind:       "Application",
		Metadata:   resource.ApplicationMeta{Name: "app", Project: "beenaire", Environment: "staging"},
		Spec:       spec,
	}
}

func dest() resource.DestinationRef {
	return resource.DestinationRef{Server: "localhost", Network: "coolify"}
}

func gitSource() *resource.SourceSpec {
	return &resource.SourceSpec{GitRepository: "https://github.com/acme/app", GitBranch: "main", PortsExposes: "3000"}
}

func TestApplicationBuildPackXORValidation(t *testing.T) {
	tests := []struct {
		name       string
		spec       resource.ApplicationSpec
		errSnippet string // empty => valid
	}{
		// dockerimage
		{
			name: "dockerimage valid",
			spec: resource.ApplicationSpec{BuildPack: "dockerimage", Image: &resource.ImageSpec{Name: "registry/app", Tag: "v1"}, Destination: dest(), Port: 8000},
		},
		{
			name:       "dockerimage missing image",
			spec:       resource.ApplicationSpec{BuildPack: "dockerimage", Destination: dest(), Port: 8000},
			errSnippet: "spec.image",
		},
		{
			name:       "dockerimage with dockerfile rejected",
			spec:       resource.ApplicationSpec{BuildPack: "dockerimage", Image: &resource.ImageSpec{Name: "r", Tag: "v1"}, Dockerfile: "FROM busybox", Destination: dest(), Port: 8000},
			errSnippet: "uses `image` only",
		},
		// dockerfile inline
		{
			name: "dockerfile inline valid",
			spec: resource.ApplicationSpec{BuildPack: "dockerfile", Dockerfile: "FROM busybox\nCMD [\"true\"]", Destination: dest()},
		},
		{
			name: "dockerfile from source valid",
			spec: resource.ApplicationSpec{BuildPack: "dockerfile", Source: gitSource(), Destination: dest()},
		},
		{
			name:       "dockerfile both inline and source rejected",
			spec:       resource.ApplicationSpec{BuildPack: "dockerfile", Dockerfile: "FROM busybox", Source: gitSource(), Destination: dest()},
			errSnippet: "exactly one",
		},
		{
			name:       "dockerfile neither inline nor source rejected",
			spec:       resource.ApplicationSpec{BuildPack: "dockerfile", Destination: dest()},
			errSnippet: "exactly one",
		},
		{
			name:       "dockerfile with image rejected",
			spec:       resource.ApplicationSpec{BuildPack: "dockerfile", Dockerfile: "FROM busybox", Image: &resource.ImageSpec{Name: "r", Tag: "v1"}, Destination: dest()},
			errSnippet: "must not set `image`",
		},
		// source-git build packs
		{
			name: "nixpacks valid",
			spec: resource.ApplicationSpec{BuildPack: "nixpacks", Source: gitSource(), Destination: dest()},
		},
		{
			name: "docker-compose valid",
			spec: resource.ApplicationSpec{BuildPack: "docker-compose", Source: gitSource(), Destination: dest()},
		},
		{
			name: "static valid",
			spec: resource.ApplicationSpec{BuildPack: "static", Source: gitSource(), Destination: dest()},
		},
		{
			name: "railpack valid",
			spec: resource.ApplicationSpec{BuildPack: "railpack", Source: gitSource(), Destination: dest()},
		},
		{
			name:       "nixpacks without source rejected",
			spec:       resource.ApplicationSpec{BuildPack: "nixpacks", Destination: dest()},
			errSnippet: "spec.source",
		},
		{
			name:       "static with image rejected",
			spec:       resource.ApplicationSpec{BuildPack: "static", Image: &resource.ImageSpec{Name: "r", Tag: "v1"}, Destination: dest()},
			errSnippet: "builds from a git `source`",
		},
		{
			name:       "railpack source missing ports_exposes rejected",
			spec:       resource.ApplicationSpec{BuildPack: "railpack", Source: &resource.SourceSpec{GitRepository: "https://github.com/acme/app", GitBranch: "main"}, Destination: dest()},
			errSnippet: "ports_exposes",
		},
		// enum
		{
			name:       "unknown build_pack rejected",
			spec:       resource.ApplicationSpec{BuildPack: "rust-magic", Destination: dest()},
			errSnippet: "build_pack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := appWith(tt.spec).Validate()
			if tt.errSnippet == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errSnippet) {
				t.Errorf("Validate() = %v, want substring %q", err, tt.errSnippet)
			}
		})
	}
}

func TestDockerfileSizeLimitRejects(t *testing.T) {
	oversize := strings.Repeat("RUN echo padding to inflate the dockerfile\n", 30_000) // > 1 MiB
	if len(oversize) <= 1<<20 {
		t.Fatalf("test fixture is %d bytes, want > 1 MiB", len(oversize))
	}
	err := appWith(resource.ApplicationSpec{BuildPack: "dockerfile", Dockerfile: oversize, Destination: dest()}).Validate()
	if err == nil || !strings.Contains(err.Error(), "exceeds 1 MB") {
		t.Errorf("Validate() = %v, want 'exceeds 1 MB'", err)
	}
	// A small Dockerfile is accepted.
	if err := appWith(resource.ApplicationSpec{BuildPack: "dockerfile", Dockerfile: "FROM busybox", Destination: dest()}).Validate(); err != nil {
		t.Errorf("small dockerfile rejected: %v", err)
	}
}

func TestGitBranchRegexRejects(t *testing.T) {
	for _, branch := range []string{"feature branch", "main;rm -rf /", "--upload-pack=evil"} {
		spec := resource.ApplicationSpec{BuildPack: "nixpacks", Source: &resource.SourceSpec{GitRepository: "https://github.com/acme/app", GitBranch: branch, PortsExposes: "3000"}, Destination: dest()}
		err := appWith(spec).Validate()
		if err == nil || !strings.Contains(err.Error(), "branch name format") {
			t.Errorf("branch %q: Validate() = %v, want 'branch name format'", branch, err)
		}
	}
	// A normal branch passes.
	if err := appWith(resource.ApplicationSpec{BuildPack: "nixpacks", Source: gitSource(), Destination: dest()}).Validate(); err != nil {
		t.Errorf("valid branch rejected: %v", err)
	}
}

func TestGitRepositorySchemeAllowlist(t *testing.T) {
	for _, repo := range []string{"javascript:alert(1)", "file:///etc/passwd", "ftp://mirror/repo.git"} {
		spec := resource.ApplicationSpec{BuildPack: "nixpacks", Source: &resource.SourceSpec{GitRepository: repo, GitBranch: "main", PortsExposes: "3000"}, Destination: dest()}
		err := appWith(spec).Validate()
		if err == nil || !strings.Contains(err.Error(), "scheme") {
			t.Errorf("repo %q: Validate() = %v, want 'scheme'", repo, err)
		}
	}
	for _, repo := range []string{"https://github.com/acme/app", "http://git.local/app", "git@github.com:acme/app.git"} {
		spec := resource.ApplicationSpec{BuildPack: "nixpacks", Source: &resource.SourceSpec{GitRepository: repo, GitBranch: "main", PortsExposes: "3000"}, Destination: dest()}
		if err := appWith(spec).Validate(); err != nil {
			t.Errorf("repo %q rejected: %v", repo, err)
		}
	}
}
