package resource

import (
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// secretType is the reflect.Type of secrets.Secret, mapped to a string schema since
// the type has no exported fields (its value is opaque by design).
var secretType = reflect.TypeOf(secrets.Secret{})

// newReflector returns a jsonschema reflector that renders secrets.Secret as a
// constrained string (${env:NAME} or ${sops:path}) rather than its opaque Go shape.
func newReflector() *jsonschema.Reflector {
	return &jsonschema.Reflector{
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
}

// ApplicationSchema returns the JSON Schema for an Application, derived from the struct
// tags (single source of truth).
func ApplicationSchema() *jsonschema.Schema { return newReflector().Reflect(&Application{}) }

// DatabaseSchema returns the JSON Schema for a Database resource.
func DatabaseSchema() *jsonschema.Schema { return newReflector().Reflect(&Database{}) }

// EnvVarSchema returns the JSON Schema for a standalone EnvVar resource.
func EnvVarSchema() *jsonschema.Schema { return newReflector().Reflect(&EnvVar{}) }

// ProjectSchema returns the JSON Schema for a Project resource.
func ProjectSchema() *jsonschema.Schema { return newReflector().Reflect(&Project{}) }

// EnvironmentSchema returns the JSON Schema for an Environment resource.
func EnvironmentSchema() *jsonschema.Schema { return newReflector().Reflect(&Environment{}) }

// ServiceSchema returns the JSON Schema for a Service resource.
func ServiceSchema() *jsonschema.Schema { return newReflector().Reflect(&Service{}) }

// Schemas maps each resource slug (matching its source file and generated docs) to its
// JSON Schema, so docs generation can emit one schema per resource.
func Schemas() map[string]*jsonschema.Schema {
	return map[string]*jsonschema.Schema{
		"application": ApplicationSchema(),
		"database":    DatabaseSchema(),
		"envvar":      EnvVarSchema(),
		"environment": EnvironmentSchema(),
		"project":     ProjectSchema(),
		"service":     ServiceSchema(),
	}
}
