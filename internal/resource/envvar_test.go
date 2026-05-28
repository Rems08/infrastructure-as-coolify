package resource_test

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

const validEnvVarYAML = `
api_version: iac-coolify/v1
kind: EnvVar
metadata:
  name: shared-secrets
  project: beenaire
  environment: staging
spec:
  vars:
    - name: SHARED_FLAG
      value: "on"
    - name: SHARED_TOKEN
      value_secret: ${env:SHARED_TOKEN}
`

func TestEnvVarValidate(t *testing.T) {
	t.Setenv("SHARED_TOKEN", "tok")
	tests := []struct {
		name       string
		yaml       string
		wantParse  bool
		errSnippet string // empty => expect valid
	}{
		{name: "valid", yaml: validEnvVarYAML, wantParse: true},
		{
			name:       "wrong kind",
			yaml:       strings.Replace(validEnvVarYAML, "kind: EnvVar", "kind: Application", 1),
			wantParse:  true,
			errSnippet: "kind",
		},
		{
			name:       "no vars",
			yaml:       "\napi_version: iac-coolify/v1\nkind: EnvVar\nmetadata:\n  name: x\n  project: p\n  environment: staging\nspec:\n  vars: []\n",
			wantParse:  true,
			errSnippet: "at least one",
		},
		{
			name:       "entry both value and secret",
			yaml:       strings.Replace(validEnvVarYAML, "      value: \"on\"", "      value: \"on\"\n      value_secret: ${env:SHARED_TOKEN}", 1),
			wantParse:  true,
			errSnippet: "exactly one",
		},
		{
			name:      "literal secret rejected at parse",
			yaml:      strings.Replace(validEnvVarYAML, "value_secret: ${env:SHARED_TOKEN}", "value_secret: s3cr3t-literal", 1),
			wantParse: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ev resource.EnvVar
			err := yaml.Unmarshal([]byte(tt.yaml), &ev)
			if !tt.wantParse {
				if err == nil {
					t.Fatal("expected a parse error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			vErr := ev.Validate()
			if tt.errSnippet == "" {
				if vErr != nil {
					t.Fatalf("Validate() = %v, want nil", vErr)
				}
				return
			}
			if vErr == nil || !strings.Contains(vErr.Error(), tt.errSnippet) {
				t.Errorf("Validate() = %v, want substring %q", vErr, tt.errSnippet)
			}
		})
	}
}

func TestEnvVarSchemaGolden(t *testing.T) {
	checkSchemaGolden(t, "envvar_schema.golden.json", resource.EnvVarSchema())
}
