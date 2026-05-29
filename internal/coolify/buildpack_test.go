package coolify

import (
	"strings"
	"testing"
)

func TestBuildPackEndpointAllCombinations(t *testing.T) {
	tests := []struct {
		name         string
		req          CreateApplicationRequest
		wantEndpoint string
		wantAPIBP    string
	}{
		{"dockerimage", CreateApplicationRequest{BuildPack: "dockerimage"}, "/applications/dockerimage", ""},
		{"dockerfile inline", CreateApplicationRequest{BuildPack: "dockerfile", Dockerfile: "FROM busybox"}, "/applications/dockerfile", "dockerfile"},
		{"dockerfile from git", CreateApplicationRequest{BuildPack: "dockerfile", GitRepository: "https://github.com/acme/app", GitBranch: "main"}, "/applications/public", "dockerfile"},
		{"nixpacks", CreateApplicationRequest{BuildPack: "nixpacks"}, "/applications/public", "nixpacks"},
		{"docker-compose", CreateApplicationRequest{BuildPack: "docker-compose"}, "/applications/public", "dockercompose"},
		{"static", CreateApplicationRequest{BuildPack: "static"}, "/applications/public", "static"},
		{"railpack", CreateApplicationRequest{BuildPack: "railpack"}, "/applications/public", "railpack"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, apiBP, err := applicationCreateEndpoint(tt.req)
			if err != nil {
				t.Fatalf("applicationCreateEndpoint(%+v) error: %v", tt.req, err)
			}
			if endpoint != tt.wantEndpoint {
				t.Errorf("endpoint = %q, want %q", endpoint, tt.wantEndpoint)
			}
			if apiBP != tt.wantAPIBP {
				t.Errorf("apiBuildPack = %q, want %q", apiBP, tt.wantAPIBP)
			}
		})
	}
}

// TestBuildPackTranslateDockerComposeIaCToCoolify pins the one build_pack whose IaC
// spelling (docker-compose) differs from the upstream Coolify enum (dockercompose).
func TestBuildPackTranslateDockerComposeIaCToCoolify(t *testing.T) {
	_, apiBP, err := applicationCreateEndpoint(CreateApplicationRequest{BuildPack: "docker-compose"})
	if err != nil {
		t.Fatal(err)
	}
	if apiBP != "dockercompose" {
		t.Errorf("IaC docker-compose must map to Coolify dockercompose, got %q", apiBP)
	}
}

func TestBuildPackEndpointUnknownIsError(t *testing.T) {
	_, _, err := applicationCreateEndpoint(CreateApplicationRequest{BuildPack: "rust-magic"})
	if err == nil {
		t.Fatal("an unknown build_pack must be an error, not a guess")
	}
	if !strings.Contains(err.Error(), "rust-magic") {
		t.Errorf("error should name the offending build_pack, got: %v", err)
	}
}

func TestValidateCreatable(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateApplicationRequest
		wantErr string // empty => no error
	}{
		{
			name: "dockerimage with image is creatable",
			req:  CreateApplicationRequest{BuildPack: "dockerimage", DockerRegistryImageName: "registry/app"},
		},
		{
			name:    "dockerimage without image",
			req:     CreateApplicationRequest{BuildPack: "dockerimage", Name: "api"},
			wantErr: "docker_registry_image_name is required",
		},
		{
			name: "dockerfile inline is creatable",
			req:  CreateApplicationRequest{BuildPack: "dockerfile", Dockerfile: "FROM busybox"},
		},
		{
			name: "dockerfile from git is creatable",
			req:  CreateApplicationRequest{BuildPack: "dockerfile", GitRepository: "https://github.com/acme/app", GitBranch: "main"},
		},
		{
			name:    "dockerfile without inline or git",
			req:     CreateApplicationRequest{BuildPack: "dockerfile", Name: "api"},
			wantErr: "inline Dockerfile",
		},
		{
			name: "nixpacks with git is creatable",
			req:  CreateApplicationRequest{BuildPack: "nixpacks", GitRepository: "https://github.com/acme/app", GitBranch: "main"},
		},
		{
			name:    "static without git",
			req:     CreateApplicationRequest{BuildPack: "static", Name: "site"},
			wantErr: "git repository",
		},
		{
			name: "railpack with git is creatable",
			req:  CreateApplicationRequest{BuildPack: "railpack", GitRepository: "https://github.com/acme/app", GitBranch: "main"},
		},
		{
			name:    "unknown build_pack",
			req:     CreateApplicationRequest{BuildPack: "wat", Name: "api"},
			wantErr: "cannot map build_pack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreatable(tt.req)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateCreatable() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("validateCreatable() = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}
