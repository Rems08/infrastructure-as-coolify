package resource

import "fmt"

// KindEnvVar is the kind discriminator for standalone EnvVar resources.
const KindEnvVar = "EnvVar"

// EnvVar is a standalone, named set of environment variables that an Application merges
// in via `env_vars_from`. It carries the same EnvVarEntry values as an Application's
// inline `env_vars`. Cross-application sharing (resolving the reference) lands in a later
// release; in this release an Application must reference an EnvVar by name explicitly.
type EnvVar struct {
	APIVersion string     `yaml:"api_version" json:"api_version" iac:"doc=API schema version (must be iac-coolify/v1),required"`
	Kind       string     `yaml:"kind" json:"kind" iac:"doc=Resource kind (must be EnvVar),required,enum=EnvVar"`
	Metadata   EnvVarMeta `yaml:"metadata" json:"metadata" iac:"doc=Identifying metadata,required"`
	Spec       EnvVarSpec `yaml:"spec" json:"spec" iac:"doc=Desired environment-variable set,required"`
}

// EnvVarMeta is the logical identity of an EnvVar resource.
type EnvVarMeta struct {
	Name        string `yaml:"name" json:"name" iac:"doc=Logical name referenced by an Application env_vars_from,required"`
	Project     string `yaml:"project" json:"project" iac:"doc=Logical project name (referenced by name),required"`
	Environment string `yaml:"environment" json:"environment" iac:"doc=Environment name such as staging or production,required"`
}

// EnvVarSpec holds the variables of an EnvVar resource.
type EnvVarSpec struct {
	Vars []EnvVarEntry `yaml:"vars" json:"vars" iac:"doc=Environment-variable entries (see Application env_vars),required"`
}

// Validate checks the EnvVar against schema rules the type system cannot express.
func (e EnvVar) Validate() error {
	if e.APIVersion != APIVersion {
		return fmt.Errorf("api_version: must be %q, got %q", APIVersion, e.APIVersion)
	}
	if e.Kind != KindEnvVar {
		return fmt.Errorf("kind: must be %q, got %q", KindEnvVar, e.Kind)
	}
	if err := e.Metadata.validate(); err != nil {
		return err
	}
	if len(e.Spec.Vars) == 0 {
		return fmt.Errorf("spec.vars: at least one variable is required")
	}
	for i := range e.Spec.Vars {
		if err := e.Spec.Vars[i].Validate(); err != nil {
			return fmt.Errorf("spec.vars[%d]: %w", i, err)
		}
	}
	return nil
}

func (m EnvVarMeta) validate() error {
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
