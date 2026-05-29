package coolify_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// secret builds a Secret carrying value, for asserting credential reveal at the boundary.
func secret(t *testing.T, value string) secrets.Secret {
	t.Helper()
	return secrets.NewRemote(value)
}

func TestCreateDatabasePostgresql(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-pg"}`)
	uuid, err := newTestClient(t, srv.URL).CreateDatabasePostgresql(context.Background(), coolify.CreateDatabasePostgresqlRequest{
		CreateDatabaseCommon: coolify.CreateDatabaseCommon{
			ServerUUID: "srv", ProjectUUID: "proj", EnvironmentName: "staging", Name: "pg", Image: "postgres:18",
		},
		PostgresUser:     "app",
		PostgresPassword: secret(t, "pg-secret"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if uuid != "db-pg" {
		t.Errorf("uuid = %q, want db-pg", uuid)
	}
	req := (*got)[0]
	if req.method != http.MethodPost || req.path != "/api/v1/databases/postgresql" {
		t.Errorf("got %s %s, want POST /api/v1/databases/postgresql", req.method, req.path)
	}
	if req.idemp == "" {
		t.Error("create must carry an Idempotency-Key")
	}
	if req.body["postgres_password"] != "pg-secret" {
		t.Errorf("password not revealed into body: %+v", req.body)
	}
	if req.body["postgres_user"] != "app" || req.body["server_uuid"] != "srv" {
		t.Errorf("body missing fields: %+v", req.body)
	}
	// environment_uuid is omitted (v4 environments have no UUID); is_public false is omitted.
	if _, ok := req.body["environment_uuid"]; ok {
		t.Errorf("environment_uuid must be omitted: %+v", req.body)
	}
	if _, ok := req.body["is_public"]; ok {
		t.Errorf("is_public false must be omitted: %+v", req.body)
	}
}

func TestCreateDatabaseMysql(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-mysql"}`)
	_, err := newTestClient(t, srv.URL).CreateDatabaseMysql(context.Background(), coolify.CreateDatabaseMysqlRequest{
		CreateDatabaseCommon: coolify.CreateDatabaseCommon{ServerUUID: "s", ProjectUUID: "p", EnvironmentName: "staging", Name: "my"},
		MySQLRootPassword:    secret(t, "root-pw"),
		MySQLPassword:        secret(t, "user-pw"),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/databases/mysql" {
		t.Errorf("path = %q", req.path)
	}
	if req.body["mysql_root_password"] != "root-pw" || req.body["mysql_password"] != "user-pw" {
		t.Errorf("both mysql credentials must be revealed: %+v", req.body)
	}
}

func TestCreateDatabaseMariadb(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-maria"}`)
	_, err := newTestClient(t, srv.URL).CreateDatabaseMariadb(context.Background(), coolify.CreateDatabaseMariadbRequest{
		CreateDatabaseCommon: coolify.CreateDatabaseCommon{ServerUUID: "s", ProjectUUID: "p", EnvironmentName: "staging", Name: "maria"},
		MariaDBRootPassword:  secret(t, "maria-root"),
		MariaDBPassword:      secret(t, "maria-user"),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/databases/mariadb" {
		t.Errorf("path = %q", req.path)
	}
	if req.body["mariadb_root_password"] != "maria-root" || req.body["mariadb_password"] != "maria-user" {
		t.Errorf("both mariadb credentials must be revealed: %+v", req.body)
	}
}

func TestCreateDatabaseMongodb(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-mongo"}`)
	_, err := newTestClient(t, srv.URL).CreateDatabaseMongodb(context.Background(), coolify.CreateDatabaseMongodbRequest{
		CreateDatabaseCommon:    coolify.CreateDatabaseCommon{ServerUUID: "s", ProjectUUID: "p", EnvironmentName: "staging", Name: "mongo"},
		MongoInitDBRootUsername: "root",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/databases/mongodb" {
		t.Errorf("path = %q", req.path)
	}
	if req.body["mongo_initdb_root_username"] != "root" {
		t.Errorf("mongo username not sent: %+v", req.body)
	}
	// The pinned spec exposes no password field on mongo create.
	if _, ok := req.body["mongo_initdb_root_password"]; ok {
		t.Errorf("mongo create must not carry a password: %+v", req.body)
	}
}

func TestCreateDatabaseRedis(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-redis"}`)
	_, err := newTestClient(t, srv.URL).CreateDatabaseRedis(context.Background(), coolify.CreateDatabaseRedisRequest{
		CreateDatabaseCommon: coolify.CreateDatabaseCommon{ServerUUID: "s", ProjectUUID: "p", EnvironmentName: "staging", Name: "redis"},
		RedisPassword:        secret(t, "redis-pw"),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/databases/redis" || req.body["redis_password"] != "redis-pw" {
		t.Errorf("redis create wrong: %s %+v", req.path, req.body)
	}
}

func TestCreateDatabaseKeydb(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-keydb"}`)
	_, err := newTestClient(t, srv.URL).CreateDatabaseKeydb(context.Background(), coolify.CreateDatabaseKeydbRequest{
		CreateDatabaseCommon: coolify.CreateDatabaseCommon{ServerUUID: "s", ProjectUUID: "p", EnvironmentName: "staging", Name: "keydb"},
		KeyDBPassword:        secret(t, "keydb-pw"),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/databases/keydb" || req.body["keydb_password"] != "keydb-pw" {
		t.Errorf("keydb create wrong: %s %+v", req.path, req.body)
	}
}

func TestCreateDatabaseDragonfly(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-df"}`)
	_, err := newTestClient(t, srv.URL).CreateDatabaseDragonfly(context.Background(), coolify.CreateDatabaseDragonflyRequest{
		CreateDatabaseCommon: coolify.CreateDatabaseCommon{ServerUUID: "s", ProjectUUID: "p", EnvironmentName: "staging", Name: "df"},
		DragonflyPassword:    secret(t, "df-pw"),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/databases/dragonfly" || req.body["dragonfly_password"] != "df-pw" {
		t.Errorf("dragonfly create wrong: %s %+v", req.path, req.body)
	}
}

func TestCreateDatabaseClickhouse(t *testing.T) {
	srv, got := captureServer(t, http.StatusCreated, `{"uuid":"db-ch"}`)
	_, err := newTestClient(t, srv.URL).CreateDatabaseClickhouse(context.Background(), coolify.CreateDatabaseClickhouseRequest{
		CreateDatabaseCommon:    coolify.CreateDatabaseCommon{ServerUUID: "s", ProjectUUID: "p", EnvironmentName: "staging", Name: "ch"},
		ClickhouseAdminUser:     "admin",
		ClickhouseAdminPassword: secret(t, "ch-pw"),
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.path != "/api/v1/databases/clickhouse" || req.body["clickhouse_admin_password"] != "ch-pw" {
		t.Errorf("clickhouse create wrong: %s %+v", req.path, req.body)
	}
}

func TestUpdateDatabase(t *testing.T) {
	srv, got := captureServer(t, http.StatusOK, `{"message":"updated"}`)
	private := false
	err := newTestClient(t, srv.URL).UpdateDatabase(context.Background(), "db-uuid", coolify.UpdateDatabaseRequest{
		Image:    "postgres:19",
		IsPublic: &private,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := (*got)[0]
	if req.method != http.MethodPatch || req.path != "/api/v1/databases/db-uuid" {
		t.Errorf("got %s %s, want PATCH /api/v1/databases/db-uuid", req.method, req.path)
	}
	if req.idemp == "" {
		t.Error("update must carry an Idempotency-Key")
	}
	if req.body["image"] != "postgres:19" || req.body["is_public"] != false {
		t.Errorf("patch body = %+v", req.body)
	}
	// Empty fields are omitted from the partial patch.
	if _, ok := req.body["name"]; ok {
		t.Errorf("empty fields must be omitted: %+v", req.body)
	}
}

func TestUpdateDatabaseValidationErrorPropagates(t *testing.T) {
	srv, _ := captureServer(t, http.StatusUnprocessableEntity, `{"message":"invalid"}`)
	if err := newTestClient(t, srv.URL).UpdateDatabase(context.Background(), "db", coolify.UpdateDatabaseRequest{Image: "x"}); err == nil {
		t.Error("a 422 on update must propagate")
	}
}

func TestDeleteDatabaseCarriesFlags(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"deleted"}`))
	}))
	t.Cleanup(srv.Close)
	err := newTestClient(t, srv.URL).DeleteDatabase(context.Background(), "db", coolify.DeleteDatabaseOptions{
		DeleteConfigurations:    true,
		DeleteVolumes:           false,
		DockerCleanup:           true,
		DeleteConnectedNetworks: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "delete_configurations=true&delete_connected_networks=false&delete_volumes=false&docker_cleanup=true"
	if rawQuery != want {
		t.Errorf("query = %q, want %q", rawQuery, want)
	}
}

func TestDeleteDatabaseDefaultsAllTrue(t *testing.T) {
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	if err := newTestClient(t, srv.URL).DeleteDatabase(context.Background(), "db", coolify.DefaultDeleteDatabaseOptions()); err != nil {
		t.Fatal(err)
	}
	want := "delete_configurations=true&delete_connected_networks=true&delete_volumes=true&docker_cleanup=true"
	if rawQuery != want {
		t.Errorf("default query = %q, want %q", rawQuery, want)
	}
}

func TestDeleteDatabaseNotFoundIsNoop(t *testing.T) {
	srv, _ := captureServer(t, http.StatusNotFound, `{"message":"not found"}`)
	if err := newTestClient(t, srv.URL).DeleteDatabase(context.Background(), "gone", coolify.DefaultDeleteDatabaseOptions()); err != nil {
		t.Errorf("deleting an absent database must be a no-op, got: %v", err)
	}
}

func TestDeleteDatabaseServerErrorPropagates(t *testing.T) {
	srv, _ := captureServer(t, http.StatusInternalServerError, `{"message":"boom"}`)
	if err := newTestClient(t, srv.URL).DeleteDatabase(context.Background(), "x", coolify.DefaultDeleteDatabaseOptions()); err == nil {
		t.Error("a 500 on delete must propagate")
	}
}
