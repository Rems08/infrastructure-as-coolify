package resource_test

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func TestEnvironmentValidate(t *testing.T) {
	base := `
api_version: iac-coolify/v1
kind: Environment
metadata:
  name: staging
  project: beenaire
spec:
  description: Staging environment
`
	tests := []struct {
		name       string
		yaml       string
		errSnippet string // empty => expect valid
	}{
		{name: "valid", yaml: base},
		{name: "valid without description", yaml: "\napi_version: iac-coolify/v1\nkind: Environment\nmetadata:\n  name: production\n  project: beenaire\n"},
		{
			name:       "wrong kind",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Project\nmetadata:\n  name: staging\n  project: beenaire\n",
			errSnippet: "kind",
		},
		{
			name:       "missing name",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Environment\nmetadata:\n  name: \"\"\n  project: beenaire\n",
			errSnippet: "metadata.name",
		},
		{
			name:       "missing project",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Environment\nmetadata:\n  name: staging\n  project: \"\"\n",
			errSnippet: "metadata.project",
		},
		{
			name:       "invalid project name",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Environment\nmetadata:\n  name: staging\n  project: Bee_naire\n",
			errSnippet: "metadata.project",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var e resource.Environment
			if err := yaml.Unmarshal([]byte(tt.yaml), &e); err != nil {
				t.Fatalf("parse: %v", err)
			}
			err := e.Validate()
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

func TestEnvironmentSchemaGolden(t *testing.T) {
	checkSchemaGolden(t, "environment_schema.golden.json", resource.EnvironmentSchema())
}
