package resource

import (
	"fmt"
	"regexp"
)

// KindProject is the kind discriminator for Project resources.
const KindProject = "Project"

// namePattern constrains logical names to a DNS-label-like shape: lowercase
// alphanumerics and hyphens, starting and ending with an alphanumeric. It is shared by
// the resources whose names become Coolify identifiers (Project, Environment).
var namePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// validateName checks a logical name against namePattern, returning a field-qualified
// error. field is the dotted path reported to the user (e.g. "metadata.name").
func validateName(field, name string) error {
	if name == "" {
		return fmt.Errorf("%s: required", field)
	}
	if !namePattern.MatchString(name) {
		return fmt.Errorf("%s: must match %s, got %q", field, namePattern.String(), name)
	}
	return nil
}

// Project describes a Coolify project managed declaratively. A project groups
// environments, which in turn group applications and databases.
type Project struct {
	APIVersion string      `yaml:"api_version" json:"api_version" iac:"doc=API schema version (must be iac-coolify/v1),required"`
	Kind       string      `yaml:"kind" json:"kind" iac:"doc=Resource kind (must be Project),required,enum=Project"`
	Metadata   ProjectMeta `yaml:"metadata" json:"metadata" iac:"doc=Identifying metadata,required"`
	Spec       ProjectSpec `yaml:"spec" json:"spec" iac:"doc=Desired project state,required"`
}

// ProjectMeta is the logical identity of a project.
type ProjectMeta struct {
	Name string `yaml:"name" json:"name" iac:"doc=Logical name used as the immutable key (lowercase alphanumerics and hyphens),required"`
}

// ProjectSpec is the desired state of a project.
type ProjectSpec struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty" iac:"doc=Human-readable project description"`
}

// Validate checks the Project against schema rules the type system cannot express.
func (p Project) Validate() error {
	if p.APIVersion != APIVersion {
		return fmt.Errorf("api_version: must be %q, got %q", APIVersion, p.APIVersion)
	}
	if p.Kind != KindProject {
		return fmt.Errorf("kind: must be %q, got %q", KindProject, p.Kind)
	}
	return validateName("metadata.name", p.Metadata.Name)
}
