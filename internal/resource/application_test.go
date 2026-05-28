package resource_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/google/go-cmp/cmp"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

var update = flag.Bool("update", false, "update golden files")

const validYAML = `
api_version: iac-coolify/v1
kind: Application
metadata:
  name: web
  project: beenaire
  environment: staging
spec:
  build_pack: dockerimage
  image:
    name: registry.gitlab.com/beenaire/web
    tag: v1-0-11
  destination:
    server: localhost
    network: coolify
  fqdn: https://app.example.com
  port: 3000
  env_vars:
    - name: NODE_ENV
      value: production
`

func TestApplicationValidate(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantParse  bool // false => expect an UnmarshalYAML/parse error
		wantValid  bool // only checked when wantParse is true
		errSnippet string
	}{
		{name: "valid", yaml: validYAML, wantParse: true, wantValid: true},
		{
			name:       "wrong api_version",
			yaml:       strings.Replace(validYAML, "iac-coolify/v1", "iac-coolify/v2", 1),
			wantParse:  true,
			errSnippet: "api_version",
		},
		{
			name:       "bad build_pack",
			yaml:       strings.Replace(validYAML, "build_pack: dockerimage", "build_pack: rust", 1),
			wantParse:  true,
			errSnippet: "build_pack",
		},
		{
			name:       "missing port",
			yaml:       strings.Replace(validYAML, "  port: 3000\n", "", 1),
			wantParse:  true,
			errSnippet: "port",
		},
		{
			name: "env_var both value and value_secret",
			yaml: strings.Replace(validYAML,
				"    - name: NODE_ENV\n      value: production\n",
				"    - name: X\n      value: a\n      value_secret: ${env:HOME}\n", 1),
			wantParse:  true,
			errSnippet: "exactly one",
		},
		{
			name: "literal secret rejected at parse",
			yaml: strings.Replace(validYAML,
				"    - name: NODE_ENV\n      value: production\n",
				"    - name: DB\n      value_secret: postgres://u:p@h/db\n", 1),
			wantParse: false,
		},
		{
			name:       "fqdn without scheme",
			yaml:       strings.Replace(validYAML, "https://app.example.com", "app.example.com", 1),
			wantParse:  true,
			errSnippet: "fqdn",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var app resource.Application
			err := yaml.Unmarshal([]byte(tt.yaml), &app)
			if !tt.wantParse {
				if err == nil {
					t.Fatal("expected a parse/unmarshal error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			vErr := app.Validate()
			if tt.wantValid {
				if vErr != nil {
					t.Fatalf("Validate() = %v, want nil", vErr)
				}
				return
			}
			if vErr == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tt.errSnippet)
			}
			if !strings.Contains(vErr.Error(), tt.errSnippet) {
				t.Errorf("Validate() = %q, want substring %q", vErr, tt.errSnippet)
			}
		})
	}
}

func TestApplicationParseRoundtrip(t *testing.T) {
	t.Setenv("DBURL", "postgres://real")
	const withSecret = validYAML + `    - name: DATABASE_URL
      value_secret: ${env:DBURL}
`
	var got resource.Application
	if err := yaml.Unmarshal([]byte(withSecret), &got); err != nil {
		t.Fatal(err)
	}
	dbSecret, err := secrets.NewFromEnv("DBURL")
	if err != nil {
		t.Fatal(err)
	}
	want := resource.Application{
		APIVersion: "iac-coolify/v1",
		Kind:       "Application",
		Metadata:   resource.ApplicationMeta{Name: "web", Project: "beenaire", Environment: "staging"},
		Spec: resource.ApplicationSpec{
			BuildPack:   "dockerimage",
			Image:       &resource.ImageSpec{Name: "registry.gitlab.com/beenaire/web", Tag: "v1-0-11"},
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
			FQDN:        "https://app.example.com",
			Port:        3000,
			EnvVars: []resource.EnvVarEntry{
				{Name: "NODE_ENV", Value: "production"},
				{Name: "DATABASE_URL", ValueSecret: dbSecret},
			},
		},
	}
	if diff := cmp.Diff(want, got, cmp.AllowUnexported(secrets.Secret{})); diff != "" {
		t.Errorf("parsed Application mismatch (-want +got):\n%s", diff)
	}
}

func TestApplicationSchemaGolden(t *testing.T) {
	schema := resource.ApplicationSchema()
	got, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	goldenPath := filepath.Join("testdata", "application_schema.golden.json")
	if *update {
		if wErr := os.WriteFile(goldenPath, got, 0o600); wErr != nil {
			t.Fatal(wErr)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("schema differs from golden; run `go test ./internal/resource -update`")
	}
}
