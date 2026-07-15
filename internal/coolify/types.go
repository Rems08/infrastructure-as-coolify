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
	Description             string `json:"description"`
	FQDN                    string `json:"fqdn"`
	BuildPack               string `json:"build_pack"`
	DockerRegistryImageName string `json:"docker_registry_image_name"`
	DockerRegistryImageTag  string `json:"docker_registry_image_tag"`
	GitBranch               string `json:"git_branch"`
	PortsExposes            string `json:"ports_exposes"`
	Status                  string `json:"status"`
	EnvironmentID           int    `json:"environment_id"`

	// Destination carries the hosting server and docker network. The payload's top-level
	// "server" field is null at runtime, so destination.server is the only reliable source
	// of the hosting server (observed 2026-06-10, testdata/application-singular.json).
	Destination Destination `json:"destination"`
}

// Project mirrors components.schemas.Project: the documented fields the UUID resolver
// needs to map a logical project name to its resources.
type Project struct {
	ID   int    `json:"id"`
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// Environment mirrors components.schemas.Environment. ID links an Application or Service
// back to its environment. The schema has no UUID: the API addresses an environment by name
// (environment_name_or_uuid).
//
// project_id is intentionally not decoded: the spec declares it, but GET
// /projects/{uuid}/environments does not populate it at runtime, so it is always zero. The
// resolver derives the owning project while enumerating each project's environments (see
// state.envRef) — do not re-add a ProjectID field and key children on it, or every
// application and service is dropped.
type Environment struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
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

	Destination Destination `json:"destination"`
}

// Destination is the subset of the nested destination object the reconciler reads: the
// docker network a resource is attached to and the server hosting it. The full object also
// carries proxy and settings blobs, which iac-coolify does not consume. Server.Name is the
// logical name compared against the desired destination — never the UUID, so manifests keep
// logical references.
type Destination struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Network string `json:"network"`
	Server  Server `json:"server"`
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
	Description             string `json:"description,omitempty"`
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
	Description             string `json:"description,omitempty"`
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
//
// IsBuildtime and IsRuntime are the scope flags the API attaches to a stored env var. The
// same key can appear once per scope (a build-only entry and a runtime-only entry), so two
// rows sharing a key but differing in scope are distinct variants, not duplicates. Both flags
// are read from the live response: the pinned OpenAPI documents the env response without them,
// so they cannot be relied on from the spec alone. The scope flags are never sent on a write
// (envWire carries key and value only).
type ServiceEnvVar struct {
	UUID        string `json:"uuid,omitempty"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	IsBuildtime bool   `json:"is_buildtime"`
	IsRuntime   bool   `json:"is_runtime"`
	IsPreview   bool   `json:"is_preview"`

	Secret secrets.Secret `json:"-"`
}

// CreateDatabaseCommon holds the fields every POST /databases/{engine} endpoint accepts.
// Each engine's request embeds it and adds the engine-specific fields. server_uuid and
// project_uuid are required by every endpoint; at least one of environment_name or
// environment_uuid must be set (v4 environments have no UUID, so only the name is sent).
type CreateDatabaseCommon struct {
	ServerUUID      string `json:"server_uuid"`
	ProjectUUID     string `json:"project_uuid"`
	EnvironmentName string `json:"environment_name,omitempty"`
	EnvironmentUUID string `json:"environment_uuid,omitempty"`
	DestinationUUID string `json:"destination_uuid,omitempty"`
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Image           string `json:"image,omitempty"`
	IsPublic        bool   `json:"is_public,omitempty"`
	PublicPort      int    `json:"public_port,omitempty"`
	LimitsCPUShares int    `json:"limits_cpu_shares,omitempty"`
	LimitsMemory    string `json:"limits_memory,omitempty"`
	InstantDeploy   bool   `json:"instant_deploy,omitempty"`
}

// CreateDatabasePostgresqlRequest is the body for POST /databases/postgresql. The password
// is a Secret with json:"-"; it is revealed into the wire body only inside this package
// (see credentials), never serialised by the redacting Secret marshaller.
type CreateDatabasePostgresqlRequest struct {
	CreateDatabaseCommon
	PostgresUser           string         `json:"postgres_user,omitempty"`
	PostgresDB             string         `json:"postgres_db,omitempty"`
	PostgresInitDBArgs     string         `json:"postgres_initdb_args,omitempty"`
	PostgresHostAuthMethod string         `json:"postgres_host_auth_method,omitempty"`
	PostgresConf           string         `json:"postgres_conf,omitempty"`
	PostgresPassword       secrets.Secret `json:"-"`
}

// CreateDatabaseMysqlRequest is the body for POST /databases/mysql.
type CreateDatabaseMysqlRequest struct {
	CreateDatabaseCommon
	MySQLUser         string         `json:"mysql_user,omitempty"`
	MySQLDatabase     string         `json:"mysql_database,omitempty"`
	MySQLConf         string         `json:"mysql_conf,omitempty"`
	MySQLRootPassword secrets.Secret `json:"-"`
	MySQLPassword     secrets.Secret `json:"-"`
}

// CreateDatabaseMariadbRequest is the body for POST /databases/mariadb.
type CreateDatabaseMariadbRequest struct {
	CreateDatabaseCommon
	MariaDBUser         string         `json:"mariadb_user,omitempty"`
	MariaDBDatabase     string         `json:"mariadb_database,omitempty"`
	MariaDBConf         string         `json:"mariadb_conf,omitempty"`
	MariaDBRootPassword secrets.Secret `json:"-"`
	MariaDBPassword     secrets.Secret `json:"-"`
}

// CreateDatabaseMongodbRequest is the body for POST /databases/mongodb. The pinned v4 spec
// exposes no password field on create (only mongo_initdb_root_username); a password is set
// later via PATCH (see UpdateDatabaseRequest).
type CreateDatabaseMongodbRequest struct {
	CreateDatabaseCommon
	MongoConf               string `json:"mongo_conf,omitempty"`
	MongoInitDBRootUsername string `json:"mongo_initdb_root_username,omitempty"`
}

// CreateDatabaseRedisRequest is the body for POST /databases/redis.
type CreateDatabaseRedisRequest struct {
	CreateDatabaseCommon
	RedisConf     string         `json:"redis_conf,omitempty"`
	RedisPassword secrets.Secret `json:"-"`
}

// CreateDatabaseKeydbRequest is the body for POST /databases/keydb.
type CreateDatabaseKeydbRequest struct {
	CreateDatabaseCommon
	KeyDBConf     string         `json:"keydb_conf,omitempty"`
	KeyDBPassword secrets.Secret `json:"-"`
}

// CreateDatabaseDragonflyRequest is the body for POST /databases/dragonfly.
type CreateDatabaseDragonflyRequest struct {
	CreateDatabaseCommon
	DragonflyPassword secrets.Secret `json:"-"`
}

// CreateDatabaseClickhouseRequest is the body for POST /databases/clickhouse.
type CreateDatabaseClickhouseRequest struct {
	CreateDatabaseCommon
	ClickhouseAdminUser     string         `json:"clickhouse_admin_user,omitempty"`
	ClickhouseAdminPassword secrets.Secret `json:"-"`
}

// UpdateDatabaseRequest is the partial body sent to PATCH /databases/{uuid}. Only the
// non-empty fields are sent. IsPublic is a pointer so toggling a database back to private
// (false) is distinguishable from "field unchanged". Credential rotation is intentionally
// absent: a secret never field-diffs, so a password change is an explicit future operation.
type UpdateDatabaseRequest struct {
	Name            string `json:"name,omitempty"`
	Description     string `json:"description,omitempty"`
	Image           string `json:"image,omitempty"`
	IsPublic        *bool  `json:"is_public,omitempty"`
	PublicPort      int    `json:"public_port,omitempty"`
	LimitsCPUShares int    `json:"limits_cpu_shares,omitempty"`
	LimitsMemory    string `json:"limits_memory,omitempty"`
}

// DeleteDatabaseOptions are the DELETE /databases/{uuid} query flags. The Coolify v4
// default for each is true; DefaultDeleteDatabaseOptions mirrors that.
type DeleteDatabaseOptions struct {
	DeleteConfigurations    bool
	DeleteVolumes           bool
	DockerCleanup           bool
	DeleteConnectedNetworks bool
}

// DefaultDeleteDatabaseOptions returns the options matching the API defaults (all true): a
// full teardown of configurations, volumes, connected networks and a docker cleanup.
func DefaultDeleteDatabaseOptions() DeleteDatabaseOptions {
	return DeleteDatabaseOptions{
		DeleteConfigurations:    true,
		DeleteVolumes:           true,
		DockerCleanup:           true,
		DeleteConnectedNetworks: true,
	}
}
