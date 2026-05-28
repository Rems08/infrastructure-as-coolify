package coolify

// Application mirrors the subset of the Coolify v4 `Application` schema that
// iac-coolify reads (components.schemas.Application); the full schema has ~90 fields.
//
// NOTE: build_pack here reflects the upstream OpenAPI enum
// (nixpacks|railpack|static|dockerfile|dockercompose) and is intentionally a plain
// string for forward-compat. The IaC→API mapping for build_pack is intentionally
// absent here — the field is exposed as-is from the upstream enum, callers should not
// depend on a translated form. The user-facing IaC enum lives in internal/resource.
type Application struct {
	UUID                    string `json:"uuid"`
	Name                    string `json:"name"`
	FQDN                    string `json:"fqdn"`
	BuildPack               string `json:"build_pack"`
	DockerRegistryImageName string `json:"docker_registry_image_name"`
	DockerRegistryImageTag  string `json:"docker_registry_image_tag"`
	GitBranch               string `json:"git_branch"`
	PortsExposes            string `json:"ports_exposes"`
	Status                  string `json:"status"`
	EnvironmentID           int    `json:"environment_id"`
}

// Project mirrors components.schemas.Project: the documented fields the UUID resolver
// needs to map a logical project name to its resources.
type Project struct {
	ID   int    `json:"id"`
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// Environment mirrors components.schemas.Environment. ProjectID links it back to a
// Project.ID; ID links an Application/Database back to its environment.
type Environment struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ProjectID int    `json:"project_id"`
}
