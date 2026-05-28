package resource

import (
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// secretType is the reflect.Type of secrets.Secret, mapped to a string schema since
// the type has no exported fields (its value is opaque by design).
var secretType = reflect.TypeOf(secrets.Secret{})

// ApplicationSchema returns the JSON Schema for an Application, derived from the
// struct tags (single source of truth). secrets.Secret is rendered as a constrained
// string (${env:NAME} or ${sops:path}) rather than its opaque Go shape.
func ApplicationSchema() *jsonschema.Schema {
	r := &jsonschema.Reflector{
		Mapper: func(t reflect.Type) *jsonschema.Schema {
			if t == secretType {
				return &jsonschema.Schema{
					Type:        "string",
					Pattern:     `^\$\{(env|sops):[^}]+\}$`,
					Description: "Sensitive value sourced from ${env:NAME} or ${sops:path}; never a literal.",
				}
			}
			return nil
		},
	}
	return r.Reflect(&Application{})
}
