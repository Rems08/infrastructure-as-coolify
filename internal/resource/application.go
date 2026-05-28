// Package resource holds the user-facing declarative resource types (the iac-coolify
// YAML schema). Field documentation lives in the `iac:"doc=..."` struct tags, which
// are the single source of truth for both the generated reference docs (internal/docs)
// and the JSON Schema (schema.go).
//
// Doc-tag grammar: `iac:"doc=<text>,<flag>,..."` where flags are `required` or
// `enum=a|b|c`. The doc text MUST NOT contain a comma (it is the comma-separated
// first segment).
package resource

import (
	"fmt"
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// APIVersion is the only accepted apiVersion value in Wave 1.
const APIVersion = "iac-coolify/v1"

// KindApplication is the kind discriminator for Application resources.
const KindApplication = "Application"

// buildPacks is the user-facing IaC enum. It intentionally differs from the upstream
// Coolify enum (see internal/coolify G-W1-build_pack-enum-mismatch); the IaC→API
// mapping is Wave 2 work.
var buildPacks = map[string]bool{
	"dockerfile":     true,
	"dockerimage":    true,
	"nixpacks":       true,
	"docker-compose": true,
}

// Application describes a Coolify application managed declaratively.
type Application struct {
	APIVersion string          `yaml:"api_version" json:"api_version" iac:"doc=API schema version (must be iac-coolify/v1),required"`
	Kind       string          `yaml:"kind" json:"kind" iac:"doc=Resource kind (must be Application),required,enum=Application"`
	Metadata   ApplicationMeta `yaml:"metadata" json:"metadata" iac:"doc=Identifying metadata,required"`
	Spec       ApplicationSpec `yaml:"spec" json:"spec" iac:"doc=Desired application state,required"`
}

// ApplicationMeta is the logical identity of an application.
type ApplicationMeta struct {
	Name        string `yaml:"name" json:"name" iac:"doc=Logical name used as the immutable key,required"`
	Project     string `yaml:"project" json:"project" iac:"doc=Logical project name (referenced by name),required"`
	Environment string `yaml:"environment" json:"environment" iac:"doc=Environment name such as staging or production,required"`
}

// ApplicationSpec is the desired state of an application.
type ApplicationSpec struct {
	BuildPack   string           `yaml:"build_pack" json:"build_pack" iac:"doc=Build pack to use,required,enum=dockerfile|dockerimage|nixpacks|docker-compose"`
	Image       *ImageSpec       `yaml:"image,omitempty" json:"image,omitempty" iac:"doc=Docker image (required when build_pack is dockerimage)"`
	Destination DestinationRef   `yaml:"destination" json:"destination" iac:"doc=Server and network reference,required"`
	FQDN        string           `yaml:"fqdn,omitempty" json:"fqdn,omitempty" iac:"doc=Public URL such as https://app.example.com"`
	Port        int              `yaml:"port" json:"port" iac:"doc=Container port exposed,required"`
	HealthCheck *HealthCheckSpec `yaml:"health_check,omitempty" json:"health_check,omitempty" iac:"doc=HTTP health check configuration"`
	Limits      *LimitsSpec      `yaml:"limits,omitempty" json:"limits,omitempty" iac:"doc=CPU and memory limits"`
	Preview     *PreviewSpec     `yaml:"preview,omitempty" json:"preview,omitempty" iac:"doc=Pull-request preview configuration"`
	EnvVars     []EnvVar         `yaml:"env_vars,omitempty" json:"env_vars,omitempty" iac:"doc=Environment variables"`
}

// ImageSpec identifies a Docker image.
type ImageSpec struct {
	Name string `yaml:"name" json:"name" iac:"doc=Docker image name including the registry path,required"`
	Tag  string `yaml:"tag" json:"tag" iac:"doc=Docker image tag,required"`
}

// DestinationRef references the target server and network by logical name.
type DestinationRef struct {
	Server  string `yaml:"server" json:"server" iac:"doc=Server name (logical reference),required"`
	Network string `yaml:"network" json:"network" iac:"doc=Docker network name,required"`
}

// HealthCheckSpec is an HTTP health check.
type HealthCheckSpec struct {
	Enabled bool   `yaml:"enabled" json:"enabled" iac:"doc=Whether the health check is enabled"`
	Path    string `yaml:"path,omitempty" json:"path,omitempty" iac:"doc=HTTP path to probe such as /health"`
}

// LimitsSpec bounds CPU and memory.
type LimitsSpec struct {
	CPUShares int    `yaml:"cpu_shares,omitempty" json:"cpu_shares,omitempty" iac:"doc=Relative CPU shares"`
	Memory    string `yaml:"memory,omitempty" json:"memory,omitempty" iac:"doc=Memory limit such as 512m"`
}

// PreviewSpec configures pull-request previews.
type PreviewSpec struct {
	URLTemplate string `yaml:"url_template,omitempty" json:"url_template,omitempty" iac:"doc=Preview URL template such as {{pr_id}}.{{domain}}"`
}

// EnvVar models a Coolify env var. Exactly one of Value / ValueSecret must be set
// (cf. secrets-policy.md §3.2): the YAML field chosen determines whether the value is
// visible (`value`) or REDACTED (`value_secret`).
type EnvVar struct {
	Name        string         `yaml:"name" json:"name" iac:"doc=Variable name,required"`
	Value       string         `yaml:"value,omitempty" json:"value,omitempty" iac:"doc=Visible value (supports ${env:VAR} interpolation); use value_secret for sensitive values"`
	ValueSecret secrets.Secret `yaml:"value_secret,omitempty" json:"value_secret,omitempty" iac:"doc=Sensitive value; MUST be ${env:NAME} or ${sops:path} and is shown as [REDACTED]"`
}

// Validate enforces ExactlyOneOf{Value, ValueSecret} (cf. secrets-policy.md §3.2).
func (e EnvVar) Validate() error {
	if e.Name == "" {
		return fmt.Errorf("env_var: name is required")
	}
	hasValue := e.Value != ""
	hasSecret := !e.ValueSecret.IsZero()
	switch {
	case hasValue && hasSecret:
		return fmt.Errorf("env_var %q: must set exactly one of `value` or `value_secret`, not both", e.Name)
	case !hasValue && !hasSecret:
		return fmt.Errorf("env_var %q: must set `value` or `value_secret`", e.Name)
	default:
		return nil
	}
}

// Validate checks the resource against the schema rules that cannot be expressed by
// the type system alone. It returns the first violation found.
func (a Application) Validate() error {
	if a.APIVersion != APIVersion {
		return fmt.Errorf("api_version: must be %q, got %q", APIVersion, a.APIVersion)
	}
	if a.Kind != KindApplication {
		return fmt.Errorf("kind: must be %q, got %q", KindApplication, a.Kind)
	}
	if err := a.Metadata.validate(); err != nil {
		return err
	}
	return a.Spec.validate()
}

func (m ApplicationMeta) validate() error {
	switch {
	case m.Name == "":
		return fmt.Errorf("metadata.name: required")
	case m.Project == "":
		return fmt.Errorf("metadata.project: required")
	case m.Environment == "":
		return fmt.Errorf("metadata.environment: required")
	default:
		return nil
	}
}

func (s ApplicationSpec) validate() error {
	if !buildPacks[s.BuildPack] {
		return fmt.Errorf("spec.build_pack: must be one of dockerfile|dockerimage|nixpacks|docker-compose, got %q", s.BuildPack)
	}
	if s.BuildPack == "dockerimage" && (s.Image == nil || s.Image.Name == "" || s.Image.Tag == "") {
		return fmt.Errorf("spec.image: name and tag are required when build_pack is dockerimage")
	}
	if s.Port <= 0 {
		return fmt.Errorf("spec.port: required and must be > 0")
	}
	if s.Destination.Server == "" || s.Destination.Network == "" {
		return fmt.Errorf("spec.destination: server and network are required")
	}
	if s.FQDN != "" && !strings.HasPrefix(s.FQDN, "http://") && !strings.HasPrefix(s.FQDN, "https://") {
		return fmt.Errorf("spec.fqdn: must start with http:// or https://, got %q", s.FQDN)
	}
	for i := range s.EnvVars {
		if err := s.EnvVars[i].Validate(); err != nil {
			return fmt.Errorf("spec.env_vars[%d]: %w", i, err)
		}
	}
	return nil
}
