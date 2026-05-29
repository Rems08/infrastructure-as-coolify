package resource

import (
	"fmt"
	"strings"
)

// KindService is the kind discriminator for Service resources.
const KindService = "Service"

// Service describes a Coolify service managed declaratively. A service is a
// docker-compose stack: either a compose file kept in the user's repository
// (docker_compose_path) or a Coolify one-click template (type). Exactly one of the two
// sources must be set.
type Service struct {
	APIVersion string      `yaml:"api_version" json:"api_version" iac:"doc=API schema version (must be iac-coolify/v1),required"`
	Kind       string      `yaml:"kind" json:"kind" iac:"doc=Resource kind (must be Service),required,enum=Service"`
	Metadata   ServiceMeta `yaml:"metadata" json:"metadata" iac:"doc=Identifying metadata,required"`
	Spec       ServiceSpec `yaml:"spec" json:"spec" iac:"doc=Desired service state,required"`
}

// ServiceMeta is the logical identity of a service.
type ServiceMeta struct {
	Name        string `yaml:"name" json:"name" iac:"doc=Logical name used as the immutable key,required"`
	Project     string `yaml:"project" json:"project" iac:"doc=Logical project name (referenced by name),required"`
	Environment string `yaml:"environment" json:"environment" iac:"doc=Environment name such as staging or production,required"`
}

// ServiceSpec is the desired state of a service. DockerComposePath and Type are the two
// mutually exclusive sources; exactly one must be set.
type ServiceSpec struct {
	Destination       ServiceDestination `yaml:"destination" json:"destination" iac:"doc=Server reference the stack runs on,required"`
	Description       string             `yaml:"description,omitempty" json:"description,omitempty" iac:"doc=Human-readable service description"`
	FQDN              string             `yaml:"fqdn,omitempty" json:"fqdn,omitempty" iac:"doc=Public URL such as https://app.example.com; Coolify binds domains per compose sub-service so this is advisory and not applied on create yet"`
	InstantDeploy     bool               `yaml:"instant_deploy,omitempty" json:"instant_deploy,omitempty" iac:"doc=Start the service immediately after creation (default false)"`
	DockerComposePath string             `yaml:"docker_compose_path,omitempty" json:"docker_compose_path,omitempty" iac:"doc=Relative path to a docker-compose.yml in the repository (mutually exclusive with type)"`
	Type              string             `yaml:"type,omitempty" json:"type,omitempty" iac:"doc=Coolify one-click template identifier such as gitea-with-mysql (mutually exclusive with docker_compose_path)"`
	EnvVars           []EnvVarEntry      `yaml:"env_vars,omitempty" json:"env_vars,omitempty" iac:"doc=Environment variables applied to the service"`
}

// ServiceDestination references the target server by logical name.
type ServiceDestination struct {
	Server string `yaml:"server" json:"server" iac:"doc=Server name (logical reference),required"`
}

// HasComposePath reports whether the service sources its stack from a repository compose
// file rather than a one-click template.
func (s ServiceSpec) HasComposePath() bool { return s.DockerComposePath != "" }

// Validate checks the Service against schema rules the type system cannot express. The
// path-traversal safety of DockerComposePath is checked separately by ValidateComposePath
// at load time, which knows the file's directory.
func (s Service) Validate() error {
	if s.APIVersion != APIVersion {
		return fmt.Errorf("api_version: must be %q, got %q", APIVersion, s.APIVersion)
	}
	if s.Kind != KindService {
		return fmt.Errorf("kind: must be %q, got %q", KindService, s.Kind)
	}
	if err := validateName("metadata.name", s.Metadata.Name); err != nil {
		return err
	}
	if err := validateName("metadata.project", s.Metadata.Project); err != nil {
		return err
	}
	if err := validateName("metadata.environment", s.Metadata.Environment); err != nil {
		return err
	}
	return s.Spec.validate()
}

func (s ServiceSpec) validate() error {
	if s.Destination.Server == "" {
		return fmt.Errorf("spec.destination.server: required")
	}
	hasPath := s.DockerComposePath != ""
	hasType := s.Type != ""
	switch {
	case hasPath && hasType:
		return fmt.Errorf("spec: must set exactly one of `docker_compose_path` or `type`, not both")
	case !hasPath && !hasType:
		return fmt.Errorf("spec: must set `docker_compose_path` or `type`")
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
