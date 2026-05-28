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
// Project.ID; ID links an Application/Database back to its environment. The schema has
// no UUID: the API addresses an environment by name (environment_name_or_uuid).
type Environment struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ProjectID int    `json:"project_id"`
}

// Server mirrors the subset of components.schemas.Server the resolver needs to map a
// logical destination server name to the UUID required by the application-create body.
type Server struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// CreateResponse is the shared 201 body of the create endpoints: a single uuid.
type CreateResponse struct {
	UUID string `json:"uuid"`
}

// CreateApplicationRequest is the body sent to one of the POST /applications/* endpoints.
// BuildPack carries the iac-coolify build_pack value (not the upstream enum); the client
// selects the endpoint and the body's build_pack from it (see buildpack.go). Fields are
// a superset of all create variants; omitempty keeps each request to the fields the
// chosen variant uses.
type CreateApplicationRequest struct {
	BuildPack               string `json:"-"` // selects endpoint; never serialised directly
	ProjectUUID             string `json:"project_uuid"`
	ServerUUID              string `json:"server_uuid"`
	EnvironmentName         string `json:"environment_name,omitempty"`
	Name                    string `json:"name,omitempty"`
	Domains                 string `json:"domains,omitempty"`
	PortsExposes            string `json:"ports_exposes,omitempty"`
	DockerRegistryImageName string `json:"docker_registry_image_name,omitempty"`
	DockerRegistryImageTag  string `json:"docker_registry_image_tag,omitempty"`
	GitRepository           string `json:"git_repository,omitempty"`
	GitBranch               string `json:"git_branch,omitempty"`
	Dockerfile              string `json:"dockerfile,omitempty"`
}

// UpdateApplicationRequest is the partial body sent to PATCH /applications/{uuid}. Only
// the non-empty fields are sent, so the engine can update just what the diff changed.
type UpdateApplicationRequest struct {
	Domains                 string `json:"domains,omitempty"`
	PortsExposes            string `json:"ports_exposes,omitempty"`
	DockerRegistryImageName string `json:"docker_registry_image_name,omitempty"`
	DockerRegistryImageTag  string `json:"docker_registry_image_tag,omitempty"`
}

// CreateProjectRequest is the body sent to POST /projects.
type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// CreateEnvironmentRequest is the body sent to POST /projects/{uuid}/environments.
type CreateEnvironmentRequest struct {
	Name string `json:"name"`
}
