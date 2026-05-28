package coolify

import (
	"strings"
	"testing"
)

func TestBuildPackMappingAllVariants(t *testing.T) {
	tests := []struct {
		buildPack    string
		wantEndpoint string
		wantAPIBP    string
	}{
		{"dockerimage", "/applications/dockerimage", ""},
		{"dockerfile", "/applications/dockerfile", "dockerfile"},
		{"nixpacks", "/applications/public", "nixpacks"},
		{"docker-compose", "/applications/public", "dockercompose"},
	}
	for _, tt := range tests {
		t.Run(tt.buildPack, func(t *testing.T) {
			endpoint, apiBP, err := ApplicationCreateEndpoint(tt.buildPack)
			if err != nil {
				t.Fatalf("ApplicationCreateEndpoint(%q) error: %v", tt.buildPack, err)
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

func TestBuildPackMappingUnknownIsError(t *testing.T) {
	_, _, err := ApplicationCreateEndpoint("rust-magic")
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
			name:    "dockerfile not creatable from schema",
			req:     CreateApplicationRequest{BuildPack: "dockerfile", Name: "api"},
			wantErr: "does not yet model",
		},
		{
			name:    "nixpacks not creatable from schema",
			req:     CreateApplicationRequest{BuildPack: "nixpacks", Name: "api"},
			wantErr: "git repository",
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
