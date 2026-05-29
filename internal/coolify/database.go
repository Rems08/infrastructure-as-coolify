package coolify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// DatabaseCreateRequest is implemented by every CreateDatabase{Engine}Request. The
// unexported methods keep the set closed to this package's engine request types, so a
// caller cannot smuggle in an arbitrary engine or leak a credential past the boundary.
type DatabaseCreateRequest interface {
	// engine selects the POST /databases/{engine} endpoint.
	engine() string
	// credentials returns the engine-specific credential fields to fold into the wire body,
	// revealing each Secret. Reveal happens only here, inside the allowlisted package.
	credentials() map[string]string
}

func (r CreateDatabasePostgresqlRequest) engine() string { return "postgresql" }
func (r CreateDatabasePostgresqlRequest) credentials() map[string]string {
	return revealOne("postgres_password", r.PostgresPassword)
}

func (r CreateDatabaseMysqlRequest) engine() string { return "mysql" }
func (r CreateDatabaseMysqlRequest) credentials() map[string]string {
	return mergeReveals(
		revealOne("mysql_root_password", r.MySQLRootPassword),
		revealOne("mysql_password", r.MySQLPassword),
	)
}

func (r CreateDatabaseMariadbRequest) engine() string { return "mariadb" }
func (r CreateDatabaseMariadbRequest) credentials() map[string]string {
	return mergeReveals(
		revealOne("mariadb_root_password", r.MariaDBRootPassword),
		revealOne("mariadb_password", r.MariaDBPassword),
	)
}

func (r CreateDatabaseMongodbRequest) engine() string                 { return "mongodb" }
func (r CreateDatabaseMongodbRequest) credentials() map[string]string { return nil }

func (r CreateDatabaseRedisRequest) engine() string { return "redis" }
func (r CreateDatabaseRedisRequest) credentials() map[string]string {
	return revealOne("redis_password", r.RedisPassword)
}

func (r CreateDatabaseKeydbRequest) engine() string { return "keydb" }
func (r CreateDatabaseKeydbRequest) credentials() map[string]string {
	return revealOne("keydb_password", r.KeyDBPassword)
}

func (r CreateDatabaseDragonflyRequest) engine() string { return "dragonfly" }
func (r CreateDatabaseDragonflyRequest) credentials() map[string]string {
	return revealOne("dragonfly_password", r.DragonflyPassword)
}

func (r CreateDatabaseClickhouseRequest) engine() string { return "clickhouse" }
func (r CreateDatabaseClickhouseRequest) credentials() map[string]string {
	return revealOne("clickhouse_admin_password", r.ClickhouseAdminPassword)
}

// revealOne returns a single-field map of the revealed secret, or nil when the secret is
// unset. It is the only place a database credential leaves its Secret.
func revealOne(field string, s secrets.Secret) map[string]string {
	if s.IsZero() {
		return nil
	}
	return map[string]string{field: s.Reveal()}
}

// mergeReveals combines per-field credential maps into one.
func mergeReveals(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// databaseCreateEndpoint maps an engine to its POST /databases/{engine} endpoint (relative
// to /api/v1). An unknown engine is an error, never a guess.
func databaseCreateEndpoint(engine string) (string, error) {
	switch engine {
	case "postgresql", "mysql", "mariadb", "mongodb", "redis", "keydb", "dragonfly", "clickhouse":
		return "/databases/" + engine, nil
	default:
		return "", fmt.Errorf(
			"coolify: cannot map database engine %q to a Coolify v4 endpoint (want postgresql|mysql|mariadb|mongodb|redis|keydb|dragonfly|clickhouse)",
			engine)
	}
}

// CreateDatabase posts req to its engine endpoint and returns the new database's UUID. The
// credential secrets are revealed into the body here, at the HTTP boundary, never earlier.
func (c *Client) CreateDatabase(ctx context.Context, req DatabaseCreateRequest) (string, error) {
	endpoint, err := databaseCreateEndpoint(req.engine())
	if err != nil {
		return "", err
	}
	body, err := databaseCreateBody(req)
	if err != nil {
		return "", err
	}
	httpReq, err := c.newWriteRequest(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	var out CreateResponse
	if err := c.doJSON(httpReq, &out, "POST "+endpoint); err != nil {
		return "", err
	}
	return out.UUID, nil
}

// databaseCreateBody marshals req (whose Secret fields are json:"-") into a JSON object and
// folds the revealed credential strings in. The revealed values exist only in the returned
// map, never on a typed field a caller could log or marshal by accident.
func databaseCreateBody(req DatabaseCreateRequest) (map[string]any, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("coolify: marshal database create body: %w", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("coolify: decode database create body: %w", err)
	}
	for k, v := range req.credentials() {
		body[k] = v
	}
	return body, nil
}

// CreateDatabasePostgresql creates a PostgreSQL database and returns its UUID.
func (c *Client) CreateDatabasePostgresql(ctx context.Context, req CreateDatabasePostgresqlRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// CreateDatabaseMysql creates a MySQL database and returns its UUID.
func (c *Client) CreateDatabaseMysql(ctx context.Context, req CreateDatabaseMysqlRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// CreateDatabaseMariadb creates a MariaDB database and returns its UUID.
func (c *Client) CreateDatabaseMariadb(ctx context.Context, req CreateDatabaseMariadbRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// CreateDatabaseMongodb creates a MongoDB database and returns its UUID.
func (c *Client) CreateDatabaseMongodb(ctx context.Context, req CreateDatabaseMongodbRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// CreateDatabaseRedis creates a Redis database and returns its UUID.
func (c *Client) CreateDatabaseRedis(ctx context.Context, req CreateDatabaseRedisRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// CreateDatabaseKeydb creates a KeyDB database and returns its UUID.
func (c *Client) CreateDatabaseKeydb(ctx context.Context, req CreateDatabaseKeydbRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// CreateDatabaseDragonfly creates a DragonFly database and returns its UUID.
func (c *Client) CreateDatabaseDragonfly(ctx context.Context, req CreateDatabaseDragonflyRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// CreateDatabaseClickhouse creates a Clickhouse database and returns its UUID.
func (c *Client) CreateDatabaseClickhouse(ctx context.Context, req CreateDatabaseClickhouseRequest) (string, error) {
	return c.CreateDatabase(ctx, req)
}

// UpdateDatabase patches the non-empty fields of req onto the database identified by uuid
// via PATCH /databases/{uuid}.
func (c *Client) UpdateDatabase(ctx context.Context, uuid string, req UpdateDatabaseRequest) error {
	httpReq, err := c.newWriteRequest(ctx, http.MethodPatch, "/databases/"+uuid, req)
	if err != nil {
		return err
	}
	return c.doJSON(httpReq, nil, "PATCH database "+uuid)
}

// DeleteDatabase deletes the database identified by uuid via DELETE /databases/{uuid},
// passing the teardown flags as query parameters. A 404 is treated as success so a repeated
// destroy is a no-op.
func (c *Client) DeleteDatabase(ctx context.Context, uuid string, opts DeleteDatabaseOptions) error {
	q := url.Values{}
	q.Set("delete_configurations", strconv.FormatBool(opts.DeleteConfigurations))
	q.Set("delete_volumes", strconv.FormatBool(opts.DeleteVolumes))
	q.Set("docker_cleanup", strconv.FormatBool(opts.DockerCleanup))
	q.Set("delete_connected_networks", strconv.FormatBool(opts.DeleteConnectedNetworks))
	httpReq, err := c.newWriteRequest(ctx, http.MethodDelete, "/databases/"+uuid+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	return ignoreNotFound(c.doJSON(httpReq, nil, "DELETE database "+uuid))
}
