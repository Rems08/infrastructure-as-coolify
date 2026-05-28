package resource

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// KindDatabase is the kind discriminator for Database resources.
const KindDatabase = "Database"

// dbEngines is the set of database engines Coolify v4 supports, mirroring the documented
// per-engine create endpoints (/databases/{engine}) in the pinned OpenAPI spec.
var dbEngines = map[string]bool{
	"postgresql": true,
	"mysql":      true,
	"mariadb":    true,
	"mongodb":    true,
	"redis":      true,
	"keydb":      true,
	"dragonfly":  true,
	"clickhouse": true,
}

// Database describes a Coolify managed database declaratively.
type Database struct {
	APIVersion string       `yaml:"api_version" json:"api_version" iac:"doc=API schema version (must be iac-coolify/v1),required"`
	Kind       string       `yaml:"kind" json:"kind" iac:"doc=Resource kind (must be Database),required,enum=Database"`
	Metadata   DatabaseMeta `yaml:"metadata" json:"metadata" iac:"doc=Identifying metadata,required"`
	Spec       DatabaseSpec `yaml:"spec" json:"spec" iac:"doc=Desired database state,required"`
}

// DatabaseMeta is the logical identity of a database.
type DatabaseMeta struct {
	Name        string `yaml:"name" json:"name" iac:"doc=Logical name used as the immutable key,required"`
	Project     string `yaml:"project" json:"project" iac:"doc=Logical project name (referenced by name),required"`
	Environment string `yaml:"environment" json:"environment" iac:"doc=Environment name such as staging or production,required"`
}

// DatabaseSpec is the desired state of a database.
type DatabaseSpec struct {
	Engine      string         `yaml:"engine" json:"engine" iac:"doc=Database engine,required,enum=postgresql|mysql|mariadb|mongodb|redis|keydb|dragonfly|clickhouse"`
	Version     string         `yaml:"version,omitempty" json:"version,omitempty" iac:"doc=Engine version used as the image tag such as 16"`
	Image       string         `yaml:"image,omitempty" json:"image,omitempty" iac:"doc=Custom Docker image overriding the default engine image"`
	Destination DestinationRef `yaml:"destination" json:"destination" iac:"doc=Server and network reference,required"`
	Public      bool           `yaml:"public,omitempty" json:"public,omitempty" iac:"doc=Whether the database is exposed on a public port"`
	PublicPort  int            `yaml:"public_port,omitempty" json:"public_port,omitempty" iac:"doc=Public port (required when public is true)"`
	Limits      *LimitsSpec    `yaml:"limits,omitempty" json:"limits,omitempty" iac:"doc=CPU and memory limits"`
	Password    secrets.Secret `yaml:"password,omitempty" json:"password,omitempty" iac:"doc=Database password; MUST be ${env:NAME} or ${sops:path} and is shown as [REDACTED]"`
}

// Validate checks the Database against schema rules the type system cannot express.
func (d Database) Validate() error {
	if d.APIVersion != APIVersion {
		return fmt.Errorf("api_version: must be %q, got %q", APIVersion, d.APIVersion)
	}
	if d.Kind != KindDatabase {
		return fmt.Errorf("kind: must be %q, got %q", KindDatabase, d.Kind)
	}
	if err := d.Metadata.validate(); err != nil {
		return err
	}
	return d.Spec.validate()
}

func (m DatabaseMeta) validate() error {
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

func (s DatabaseSpec) validate() error {
	if !dbEngines[s.Engine] {
		return fmt.Errorf("spec.engine: must be one of %s, got %q", strings.Join(sortedEngines(), "|"), s.Engine)
	}
	if s.Destination.Server == "" || s.Destination.Network == "" {
		return fmt.Errorf("spec.destination: server and network are required")
	}
	if s.Public && s.PublicPort <= 0 {
		return fmt.Errorf("spec.public_port: required and must be > 0 when public is true")
	}
	if !s.Public && s.PublicPort != 0 {
		return fmt.Errorf("spec.public_port: set only when public is true")
	}
	return nil
}

func sortedEngines() []string {
	out := make([]string, 0, len(dbEngines))
	for e := range dbEngines {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}
