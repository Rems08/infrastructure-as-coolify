package config

import (
	"strconv"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func TestResolveEnvInterpolation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		env     map[string]string
		want    string
		wantErr bool
	}{
		{name: "literal unchanged", raw: "v1-0-11", want: "v1-0-11"},
		{name: "empty unchanged", raw: "", want: ""},
		{
			name: "single env resolved",
			raw:  "${env:NODE_ENV}", env: map[string]string{"NODE_ENV": "production"},
			want: "production",
		},
		{
			name: "multiple env resolved",
			raw:  "${env:A}-${env:B}", env: map[string]string{"A": "x", "B": "y"},
			want: "x-y",
		},
		{
			name: "env inside literal",
			raw:  "https://${env:HOST}/path", env: map[string]string{"HOST": "example.com"},
			want: "https://example.com/path",
		},
		{
			name: "unset env errors",
			raw:  "${env:UNSET_VAR}", wantErr: true,
		},
		{
			name: "one set one unset errors",
			raw:  "${env:SET}-${env:MISSING}", env: map[string]string{"SET": "ok"}, wantErr: true,
		},
		{name: "lowercase name not matched", raw: "${env:lower}", want: "${env:lower}"},
		{name: "spaced name not matched", raw: "${env:bad name}", want: "${env:bad name}"},
		{name: "no brace not matched", raw: "env:FOO", want: "env:FOO"},
		{
			name: "int-castable value",
			raw:  "${env:REPLICAS}", env: map[string]string{"REPLICAS": "5"},
			want: "5",
		},
		{name: "digits-in-name", raw: "${env:VAR1}", env: map[string]string{"VAR1": "z"}, want: "z"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got, err := ResolveEnvInterpolation(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveEnvInterpolation(%q): want error, got %q", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveEnvInterpolation(%q): unexpected error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Errorf("ResolveEnvInterpolation(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// TestInterpolationGenericParamFields asserts ${env:VAR} is resolved across every visible
// (Param) string field of an application — metadata, image, fqdn, source, destination,
// dockerfile, limits — and that an unset reference errors.
func TestInterpolationGenericParamFields(t *testing.T) {
	full := func() *resource.Application {
		return &resource.Application{
			Metadata: resource.ApplicationMeta{Name: "${env:NAME}", Project: "${env:PROJ}", Environment: "${env:ENVN}"},
			Spec: resource.ApplicationSpec{
				Image:       &resource.ImageSpec{Name: "${env:IMG}", Tag: "${env:TAG}"},
				FQDN:        "https://${env:HOST}:${env:PORT}",
				Dockerfile:  "FROM ${env:BASE}",
				Source:      &resource.SourceSpec{GitRepository: "${env:REPO}", GitBranch: "${env:BRANCH}", PortsExposes: "${env:EXPOSE}"},
				Destination: resource.DestinationRef{Server: "${env:SRV}", Network: "${env:NET}"},
				Limits:      &resource.LimitsSpec{Memory: "${env:MEM}"},
			},
		}
	}
	env := map[string]string{
		"NAME": "api", "PROJ": "beenaire", "ENVN": "staging",
		"IMG": "registry/api", "TAG": "v1", "HOST": "api.example.com", "PORT": "443",
		"BASE": "busybox", "REPO": "https://github.com/acme/app", "BRANCH": "main",
		"EXPOSE": "3000", "SRV": "localhost", "NET": "coolify", "MEM": "512m",
	}

	tests := []struct {
		name string
		get  func(*resource.Application) string
		want string
	}{
		{"metadata.name", func(a *resource.Application) string { return a.Metadata.Name }, "api"},
		{"metadata.project", func(a *resource.Application) string { return a.Metadata.Project }, "beenaire"},
		{"metadata.environment", func(a *resource.Application) string { return a.Metadata.Environment }, "staging"},
		{"image.name", func(a *resource.Application) string { return a.Spec.Image.Name }, "registry/api"},
		{"image.tag", func(a *resource.Application) string { return a.Spec.Image.Tag }, "v1"},
		{"fqdn multi-ref", func(a *resource.Application) string { return a.Spec.FQDN }, "https://api.example.com:443"},
		{"dockerfile", func(a *resource.Application) string { return a.Spec.Dockerfile }, "FROM busybox"},
		{"source.git_repository", func(a *resource.Application) string { return a.Spec.Source.GitRepository }, "https://github.com/acme/app"},
		{"source.git_branch", func(a *resource.Application) string { return a.Spec.Source.GitBranch }, "main"},
		{"source.ports_exposes", func(a *resource.Application) string { return a.Spec.Source.PortsExposes }, "3000"},
		{"destination.server", func(a *resource.Application) string { return a.Spec.Destination.Server }, "localhost"},
		{"destination.network", func(a *resource.Application) string { return a.Spec.Destination.Network }, "coolify"},
		{"limits.memory", func(a *resource.Application) string { return a.Spec.Limits.Memory }, "512m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range env {
				t.Setenv(k, v)
			}
			app := full()
			if err := interpolateApplicationFields(app); err != nil {
				t.Fatalf("interpolateApplicationFields: %v", err)
			}
			if got := tt.get(app); got != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, got, tt.want)
			}
		})
	}

	t.Run("unset reference errors", func(t *testing.T) {
		app := &resource.Application{Metadata: resource.ApplicationMeta{Name: "${env:DEFINITELY_UNSET_W4}"}}
		if err := interpolateApplicationFields(app); err == nil {
			t.Fatal("expected an error for an unset env reference")
		}
	})

	t.Run("service fields resolved", func(t *testing.T) {
		t.Setenv("SVC_NAME", "grafana")
		t.Setenv("SVC_TYPE", "grafana")
		svc := &resource.Service{
			Metadata: resource.ServiceMeta{Name: "${env:SVC_NAME}"},
			Spec:     resource.ServiceSpec{Type: "${env:SVC_TYPE}"},
		}
		if err := interpolateServiceFields(svc); err != nil {
			t.Fatal(err)
		}
		if svc.Metadata.Name != "grafana" || svc.Spec.Type != "grafana" {
			t.Errorf("service fields not resolved: %+v", svc)
		}
	})
}

// TestResolveEnvInterpolation_IntCast verifies the downstream int-parse contract:
// "${env:REPLICAS}" resolves to "5" then parses to 5.
func TestResolveEnvInterpolation_IntCast(t *testing.T) {
	t.Setenv("REPLICAS", "5")
	got, err := ResolveEnvInterpolation("${env:REPLICAS}")
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("Atoi(%q): %v", got, err)
	}
	if n != 5 {
		t.Errorf("got %d, want 5", n)
	}
}
