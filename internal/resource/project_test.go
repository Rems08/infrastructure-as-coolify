package resource_test

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func TestProjectValidate(t *testing.T) {
	base := `
api_version: iac-coolify/v1
kind: Project
metadata:
  name: beenaire
spec:
  description: Beenaire platform
`
	tests := []struct {
		name       string
		yaml       string
		errSnippet string // empty => expect valid
	}{
		{name: "valid", yaml: base},
		{name: "valid without description", yaml: "\napi_version: iac-coolify/v1\nkind: Project\nmetadata:\n  name: labs\n"},
		{name: "valid single char", yaml: "\napi_version: iac-coolify/v1\nkind: Project\nmetadata:\n  name: a\n"},
		{
			name:       "wrong api_version",
			yaml:       "\napi_version: iac-coolify/v2\nkind: Project\nmetadata:\n  name: beenaire\n",
			errSnippet: "api_version",
		},
		{
			name:       "wrong kind",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Application\nmetadata:\n  name: beenaire\n",
			errSnippet: "kind",
		},
		{
			name:       "missing name",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Project\nmetadata:\n  name: \"\"\n",
			errSnippet: "metadata.name",
		},
		{
			name:       "uppercase name rejected",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Project\nmetadata:\n  name: Beenaire\n",
			errSnippet: "metadata.name",
		},
		{
			name:       "leading hyphen rejected",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Project\nmetadata:\n  name: -bee\n",
			errSnippet: "metadata.name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p resource.Project
			if err := yaml.Unmarshal([]byte(tt.yaml), &p); err != nil {
				t.Fatalf("parse: %v", err)
			}
			err := p.Validate()
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

func TestProjectSchemaGolden(t *testing.T) {
	checkSchemaGolden(t, "project_schema.golden.json", resource.ProjectSchema())
}
