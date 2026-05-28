package resource

import "fmt"

// KindEnvironment is the kind discriminator for Environment resources.
const KindEnvironment = "Environment"

// Environment describes a Coolify environment managed declaratively. An environment
// belongs to a project (referenced by name) and groups its applications and databases.
type Environment struct {
	APIVersion string          `yaml:"api_version" json:"api_version" iac:"doc=API schema version (must be iac-coolify/v1),required"`
	Kind       string          `yaml:"kind" json:"kind" iac:"doc=Resource kind (must be Environment),required,enum=Environment"`
	Metadata   EnvironmentMeta `yaml:"metadata" json:"metadata" iac:"doc=Identifying metadata,required"`
	Spec       EnvironmentSpec `yaml:"spec" json:"spec" iac:"doc=Desired environment state,required"`
}

// EnvironmentMeta is the logical identity of an environment.
type EnvironmentMeta struct {
	Name    string `yaml:"name" json:"name" iac:"doc=Logical name such as staging or production (lowercase alphanumerics and hyphens),required"`
	Project string `yaml:"project" json:"project" iac:"doc=Logical project name this environment belongs to (referenced by name),required"`
}

// EnvironmentSpec is the desired state of an environment.
type EnvironmentSpec struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty" iac:"doc=Human-readable environment description"`
}

// Validate checks the Environment against schema rules the type system cannot express.
func (e Environment) Validate() error {
	if e.APIVersion != APIVersion {
		return fmt.Errorf("api_version: must be %q, got %q", APIVersion, e.APIVersion)
	}
	if e.Kind != KindEnvironment {
		return fmt.Errorf("kind: must be %q, got %q", KindEnvironment, e.Kind)
	}
	if err := validateName("metadata.name", e.Metadata.Name); err != nil {
		return err
	}
	return validateName("metadata.project", e.Metadata.Project)
}
