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

// APIVersion is the only accepted apiVersion value.
const APIVersion = "iac-coolify/v1"

// KindApplication is the kind discriminator for Application resources.
const KindApplication = "Application"

// buildPacks is the user-facing IaC enum. It intentionally differs from the upstream
// Coolify enum (see internal/coolify); the IaC→API mapping is applied at reconciliation
// time, not stored here.
var buildPacks = map[string]bool{
	"dockerfile":     true,
	"dockerimage":    true,
	"nixpacks":       true,
	"docker-compose": true,
	"static":         true,
	"railpack":       true,
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
	BuildPack   string           `yaml:"build_pack" json:"build_pack" iac:"doc=Build pack to use,required,enum=dockerfile|dockerimage|nixpacks|docker-compose|static|railpack"`
	Image       *ImageSpec       `yaml:"image,omitempty" json:"image,omitempty" iac:"doc=Docker image (required when build_pack is dockerimage)"`
	Dockerfile  string           `yaml:"dockerfile,omitempty" json:"dockerfile,omitempty" iac:"doc=Inline Dockerfile content for build_pack dockerfile in inline mode; mutually exclusive with source"`
	Source      *SourceSpec      `yaml:"source,omitempty" json:"source,omitempty" iac:"doc=Public git source for build_pack dockerfile/nixpacks/docker-compose/static/railpack"`
	Destination DestinationRef   `yaml:"destination" json:"destination" iac:"doc=Server and network reference,required"`
	FQDN        string           `yaml:"fqdn,omitempty" json:"fqdn,omitempty" iac:"doc=Public URL such as https://app.example.com"`
	Port        int              `yaml:"port,omitempty" json:"port,omitempty" iac:"doc=Container port exposed (required for build_pack dockerimage)"`
	HealthCheck *HealthCheckSpec `yaml:"health_check,omitempty" json:"health_check,omitempty" iac:"doc=HTTP health check configuration"`
	Limits      *LimitsSpec      `yaml:"limits,omitempty" json:"limits,omitempty" iac:"doc=CPU and memory limits"`
	Preview     *PreviewSpec     `yaml:"preview,omitempty" json:"preview,omitempty" iac:"doc=Pull-request preview configuration"`
	EnvVars     []EnvVarEntry    `yaml:"env_vars,omitempty" json:"env_vars,omitempty" iac:"doc=Inline environment variables"`
	EnvVarsFrom []string         `yaml:"env_vars_from,omitempty" json:"env_vars_from,omitempty" iac:"doc=Names of EnvVar resources whose variables are merged into this application"`
}

// ImageSpec identifies a Docker image.
type ImageSpec struct {
	Name string `yaml:"name" json:"name" iac:"doc=Docker image name including the registry path,required"`
	Tag  string `yaml:"tag" json:"tag" iac:"doc=Docker image tag,required"`
}

// SourceSpec is a public git source for a build_pack that builds from a repository
// (dockerfile-from-git, nixpacks, docker-compose, static, railpack).
type SourceSpec struct {
	GitRepository string `yaml:"git_repository" json:"git_repository" iac:"doc=Public git repository URL (https:// http:// or git@),required"`
	GitBranch     string `yaml:"git_branch" json:"git_branch" iac:"doc=Git branch to deploy,required"`
	PortsExposes  string `yaml:"ports_exposes" json:"ports_exposes" iac:"doc=Ports the build exposes such as 3000,required"`
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

// EnvVarEntry models a single Coolify env var. Exactly one of Value / ValueSecret must
// be set: the YAML field chosen determines whether the value is visible (`value`) or
// REDACTED (`value_secret`). It is shared by an Application's inline `env_vars` and by
// the standalone EnvVar resource (see envvar.go).
type EnvVarEntry struct {
	Name        string         `yaml:"name" json:"name" iac:"doc=Variable name,required"`
	Value       string         `yaml:"value,omitempty" json:"value,omitempty" iac:"doc=Visible value (supports ${env:VAR} interpolation); use value_secret for sensitive values"`
	ValueSecret secrets.Secret `yaml:"value_secret,omitempty" json:"value_secret,omitempty" iac:"doc=Sensitive value; MUST be ${env:NAME} or ${sops:path} and is shown as [REDACTED]"`
}

// Validate enforces ExactlyOneOf{Value, ValueSecret}.
func (e EnvVarEntry) Validate() error {
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
		return fmt.Errorf("spec.build_pack: must be one of dockerfile|dockerimage|nixpacks|docker-compose|static|railpack, got %q", s.BuildPack)
	}
	if s.Destination.Server == "" || s.Destination.Network == "" {
		return fmt.Errorf("spec.destination: server and network are required")
	}
	if s.FQDN != "" && !strings.HasPrefix(s.FQDN, "http://") && !strings.HasPrefix(s.FQDN, "https://") {
		return fmt.Errorf("spec.fqdn: must start with http:// or https://, got %q", s.FQDN)
	}
	if err := s.validateBuildSource(); err != nil {
		return err
	}
	return s.validateEnvVars()
}

// validateBuildSource enforces, per build_pack, exactly one source of truth for the build:
// dockerimage builds from `image`; dockerfile from either inline `dockerfile` content or a
// git `source` (exactly one); every other build_pack from a git `source`. It rejects the
// fields the chosen build_pack must not carry, so a malformed combination fails at parse
// time rather than at apply time.
func (s ApplicationSpec) validateBuildSource() error {
	switch s.BuildPack {
	case "dockerimage":
		return s.validateImageMode()
	case "dockerfile":
		return s.validateDockerfileMode()
	default:
		return s.validateSourceMode()
	}
}

func (s ApplicationSpec) validateImageMode() error {
	if s.Dockerfile != "" || s.Source != nil {
		return fmt.Errorf("spec: build_pack dockerimage uses `image` only; remove `dockerfile` and `source`")
	}
	if s.Image == nil || s.Image.Name == "" || s.Image.Tag == "" {
		return fmt.Errorf("spec.image: name and tag are required when build_pack is dockerimage")
	}
	if s.Port <= 0 {
		return fmt.Errorf("spec.port: required and must be > 0 when build_pack is dockerimage")
	}
	return nil
}

func (s ApplicationSpec) validateDockerfileMode() error {
	if s.Image != nil {
		return fmt.Errorf("spec: build_pack dockerfile must not set `image`")
	}
	hasInline := s.Dockerfile != ""
	hasSource := s.Source != nil
	if hasInline == hasSource {
		return fmt.Errorf("spec: build_pack dockerfile requires exactly one of `dockerfile` (inline) or `source` (git)")
	}
	if hasInline {
		return validateDockerfile(s.Dockerfile)
	}
	return s.Source.validate()
}

func (s ApplicationSpec) validateSourceMode() error {
	if s.Image != nil || s.Dockerfile != "" {
		return fmt.Errorf("spec: build_pack %q builds from a git `source`; remove `image` and `dockerfile`", s.BuildPack)
	}
	if s.Source == nil {
		return fmt.Errorf("spec.source: git_repository, git_branch and ports_exposes are required when build_pack is %q", s.BuildPack)
	}
	return s.Source.validate()
}

func (s ApplicationSpec) validateEnvVars() error {
	for i := range s.EnvVars {
		if err := s.EnvVars[i].Validate(); err != nil {
			return fmt.Errorf("spec.env_vars[%d]: %w", i, err)
		}
	}
	for i, name := range s.EnvVarsFrom {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("spec.env_vars_from[%d]: name must not be empty", i)
		}
	}
	return nil
}
