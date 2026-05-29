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

// Database mirrors the JSON object returned by GET /databases/{uuid}.
//
// The OpenAPI v4 spec marks this endpoint's response as a string placeholder
// ("Content is very complex. Will be implemented later."), but the live API returns a
// fully typed object equivalent to GET /applications/{uuid}. The shape is decoded against
// the observed runtime response (testdata/database-singular.json, observed 2026-05-29) and
// tracked upstream at https://github.com/coollabsio/coolify/issues/10449.
//
// Credential fields (passwords and the connection URLs that embed them) are typed
// secrets.Secret so a value read back from the API is opaque the moment it is decoded:
// it never reaches a log, plan output, or error in clear text.
type Database struct {
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
	ConfigHash   string `json:"config_hash"`
	DatabaseType string `json:"database_type"`
	Description  string `json:"description"`
	Image        string `json:"image"`

	IsPublic   bool   `json:"is_public"`
	PublicPort int    `json:"public_port"`
	EnableSSL  bool   `json:"enable_ssl"`
	SSLMode    string `json:"ssl_mode"`

	EnvironmentID int `json:"environment_id"`

	LimitsCPUShares         int    `json:"limits_cpu_shares"`
	LimitsCPUs              string `json:"limits_cpus"`
	LimitsCPUset            string `json:"limits_cpuset"`
	LimitsMemory            string `json:"limits_memory"`
	LimitsMemoryReservation string `json:"limits_memory_reservation"`
	LimitsMemorySwap        string `json:"limits_memory_swap"`
	LimitsMemorySwappiness  int    `json:"limits_memory_swappiness"`

	PostgresUser           string         `json:"postgres_user"`
	PostgresDB             string         `json:"postgres_db"`
	PostgresConf           string         `json:"postgres_conf"`
	PostgresHostAuthMethod string         `json:"postgres_host_auth_method"`
	PostgresInitDBArgs     string         `json:"postgres_initdb_args"`
	PostgresPassword       secrets.Secret `json:"postgres_password"`

	RedisPassword           secrets.Secret `json:"redis_password"`
	MySQLUser               string         `json:"mysql_user"`
	MySQLDatabase           string         `json:"mysql_database"`
	MySQLPassword           secrets.Secret `json:"mysql_password"`
	MySQLRootPassword       secrets.Secret `json:"mysql_root_password"`
	MariaDBUser             string         `json:"mariadb_user"`
	MariaDBDatabase         string         `json:"mariadb_database"`
	MariaDBPassword         secrets.Secret `json:"mariadb_password"`
	MariaDBRootPassword     secrets.Secret `json:"mariadb_root_password"`
	MongoInitDBRootUsername string         `json:"mongo_initdb_root_username"`
	MongoInitDBRootPassword secrets.Secret `json:"mongo_initdb_root_password"`
	MongoInitDBDatabase     string         `json:"mongo_initdb_database"`
	KeyDBPassword           secrets.Secret `json:"keydb_password"`
	ClickhouseAdminUser     string         `json:"clickhouse_admin_user"`
	ClickhouseAdminPassword secrets.Secret `json:"clickhouse_admin_password"`
	DragonflyPassword       secrets.Secret `json:"dragonfly_password"`

	InternalDBURL secrets.Secret `json:"internal_db_url"`
	ExternalDBURL secrets.Secret `json:"external_db_url"`

	Status       string `json:"status"`
	ServerStatus bool   `json:"server_status"`
	LastOnlineAt string `json:"last_online_at"`
	RestartCount int    `json:"restart_count"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`

	Destination DatabaseDestination `json:"destination"`
}

// DatabaseDestination is the subset of the nested destination object the reconciler reads:
// the network a database is attached to. The full object also carries the server and its
// settings, which iac-coolify does not consume.
type DatabaseDestination struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Network string `json:"network"`
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
