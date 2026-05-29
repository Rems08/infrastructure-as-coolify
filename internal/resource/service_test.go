package resource_test

import (
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func TestServiceExactlyOneOfMode(t *testing.T) {
	t.Setenv("GRAFANA_ADMIN_PASSWORD", "from-env")
	tests := []struct {
		name       string
		yaml       string
		errSnippet string // empty => expect valid
	}{
		{
			name: "valid compose_path mode",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: observability-stack
  project: beenaire
  environment: production
spec:
  destination:
    server: localhost
  docker_compose_path: compose/observability.yml
`,
		},
		{
			name: "valid type mode",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: gitea
  project: beenaire
  environment: staging
spec:
  destination:
    server: localhost
  type: gitea-with-mysql
`,
		},
		{
			name: "valid with fqdn and env_vars",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: observability-stack
  project: beenaire
  environment: production
spec:
  destination:
    server: localhost
  fqdn: https://observability.beenaire.com
  instant_deploy: false
  docker_compose_path: compose/observability.yml
  env_vars:
    - name: GRAFANA_ADMIN_USER
      value: admin
    - name: GRAFANA_ADMIN_PASSWORD
      value_secret: "${env:GRAFANA_ADMIN_PASSWORD}"
`,
		},
		{
			name: "both modes rejected",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: svc
  project: beenaire
  environment: staging
spec:
  destination:
    server: localhost
  docker_compose_path: compose/x.yml
  type: gitea-with-mysql
`,
			errSnippet: "exactly one",
		},
		{
			name: "no mode rejected",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: svc
  project: beenaire
  environment: staging
spec:
  destination:
    server: localhost
`,
			errSnippet: "docker_compose_path` or `type`",
		},
		{
			name: "missing server rejected",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: svc
  project: beenaire
  environment: staging
spec:
  destination:
    server: ""
  type: gitea-with-mysql
`,
			errSnippet: "destination.server",
		},
		{
			name: "wrong kind rejected",
			yaml: `
api_version: iac-coolify/v1
kind: Application
metadata:
  name: svc
  project: beenaire
  environment: staging
spec:
  destination:
    server: localhost
  type: gitea-with-mysql
`,
			errSnippet: "kind",
		},
		{
			name: "bad fqdn rejected",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: svc
  project: beenaire
  environment: staging
spec:
  destination:
    server: localhost
  type: gitea-with-mysql
  fqdn: observability.beenaire.com
`,
			errSnippet: "spec.fqdn",
		},
		{
			name: "secret env_var with plaintext value_secret rejected at parse",
			yaml: `
api_version: iac-coolify/v1
kind: Service
metadata:
  name: svc
  project: beenaire
  environment: staging
spec:
  destination:
    server: localhost
  type: gitea-with-mysql
  env_vars:
    - name: SECRET
      value_secret: "plaintext-not-allowed"
`,
			errSnippet: "parse",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var svc resource.Service
			if err := yaml.Unmarshal([]byte(tt.yaml), &svc); err != nil {
				if tt.errSnippet == "parse" {
					return // secret literal is rejected by the Secret unmarshaller
				}
				t.Fatalf("parse: %v", err)
			}
			err := svc.Validate()
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

func TestServiceSchemaGolden(t *testing.T) {
	checkSchemaGolden(t, "service_schema.golden.json", resource.ServiceSchema())
}
