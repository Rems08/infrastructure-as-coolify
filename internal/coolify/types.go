package coolify

import "github.com/Rems08/infrastructure-as-coolify/internal/secrets"

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

// ServerResource is one item of the GET /servers/{uuid}/resources array. The Type field
// discriminates the resource kind; observed runtime values 2026-05-29 are "application",
// "service", and "standalone-<engine>" (e.g. standalone-postgresql, standalone-redis).
// The resolver uses this typed endpoint to discover databases, because the dedicated
// listing endpoints (GET /databases, GET /resources) are placeholders in the pinned spec
// (coollabsio/coolify#10449).
type ServerResource struct {
	ID        int    `json:"id"`
	UUID      string `json:"uuid"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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

// Service mirrors the subset of components.schemas.Service the resolver and reconciler
// read. EnvironmentID links it back to its environment (and thereby its project), exactly
// like Application.EnvironmentID.
type Service struct {
	UUID          string `json:"uuid"`
	Name          string `json:"name"`
	EnvironmentID int    `json:"environment_id"`
}

// CreateServiceRequest is the body sent to POST /services. Exactly one of Type (one-click
// template) or DockerComposeRaw (a repository compose file) carries the stack source.
// DockerComposeRaw holds the decoded compose content; the client base64-encodes it into
// the wire field, so the decoded form never has to be assembled at the call site.
type CreateServiceRequest struct {
	Type            string `json:"type,omitempty"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	ProjectUUID     string `json:"project_uuid"`
	EnvironmentName string `json:"environment_name,omitempty"`
	EnvironmentUUID string `json:"environment_uuid,omitempty"`
	ServerUUID      string `json:"server_uuid"`
	InstantDeploy   bool   `json:"instant_deploy"`

	DockerComposeRaw string `json:"-"` // decoded; base64-encoded by the client
}

// UpdateServiceRequest is the partial body sent to PATCH /services/{uuid}. DockerComposeRaw
// holds the decoded compose content; the client base64-encodes it.
type UpdateServiceRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`

	DockerComposeRaw string `json:"-"` // decoded; base64-encoded by the client
}

// ServiceEnvVar is a service environment variable. When Secret is set it is revealed at
// the HTTP boundary in place of Value, so a sensitive value never has to be carried as a
// plain string by the caller.
type ServiceEnvVar struct {
	UUID   string         `json:"uuid,omitempty"`
	Key    string         `json:"key"`
	Value  string         `json:"value"`
	Secret secrets.Secret `json:"-"`
}
