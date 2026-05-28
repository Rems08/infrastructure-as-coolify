package resource_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func dbYAML(engine string) string {
	return `
api_version: iac-coolify/v1
kind: Database
metadata:
  name: app-db
  project: beenaire
  environment: staging
spec:
  engine: ` + engine + `
  version: "16"
  destination:
    server: localhost
    network: coolify
`
}

// TestDatabaseValidatesAllEightEngines covers critère §7 #24: each of the 8 Coolify v4
// engines validates, and an unknown engine is rejected.
func TestDatabaseValidatesAllEightEngines(t *testing.T) {
	engines := []string{
		"postgresql", "mysql", "mariadb", "mongodb",
		"redis", "keydb", "dragonfly", "clickhouse",
	}
	for _, eng := range engines {
		t.Run("valid/"+eng, func(t *testing.T) {
			var db resource.Database
			if err := yaml.Unmarshal([]byte(dbYAML(eng)), &db); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := db.Validate(); err != nil {
				t.Errorf("engine %q should be valid: %v", eng, err)
			}
		})
	}
	for _, eng := range []string{"sqlite", "cassandra", "", "PostgreSQL"} {
		t.Run("invalid/"+eng, func(t *testing.T) {
			var db resource.Database
			if err := yaml.Unmarshal([]byte(dbYAML(eng)), &db); err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := db.Validate(); err == nil {
				t.Errorf("engine %q should be rejected", eng)
			}
		})
	}
}

func TestDatabaseValidate(t *testing.T) {
	base := dbYAML("postgresql")
	tests := []struct {
		name       string
		yaml       string
		wantParse  bool
		errSnippet string // empty => expect valid
	}{
		{name: "valid", yaml: base, wantParse: true},
		{
			name:       "missing destination",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Database\nmetadata:\n  name: db\n  project: p\n  environment: staging\nspec:\n  engine: redis\n",
			wantParse:  true,
			errSnippet: "destination",
		},
		{
			name:       "public without port",
			yaml:       base + "  public: true\n",
			wantParse:  true,
			errSnippet: "public_port",
		},
		{
			name:       "public_port without public",
			yaml:       base + "  public_port: 5432\n",
			wantParse:  true,
			errSnippet: "public_port",
		},
		{
			name:      "valid public with port",
			yaml:      base + "  public: true\n  public_port: 5432\n",
			wantParse: true,
		},
		{
			name:       "literal password rejected at parse",
			yaml:       base + "  password: hunter2\n",
			wantParse:  false,
			errSnippet: "literal",
		},
		{
			name:       "wrong kind",
			yaml:       "\napi_version: iac-coolify/v1\nkind: Application\nmetadata:\n  name: db\n  project: p\n  environment: staging\nspec:\n  engine: redis\n  destination:\n    server: localhost\n    network: coolify\n",
			wantParse:  true,
			errSnippet: "kind",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var db resource.Database
			err := yaml.Unmarshal([]byte(tt.yaml), &db)
			if !tt.wantParse {
				if err == nil {
					t.Fatal("expected a parse error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}
			vErr := db.Validate()
			if tt.errSnippet == "" {
				if vErr != nil {
					t.Fatalf("Validate() = %v, want nil", vErr)
				}
				return
			}
			if vErr == nil || !strings.Contains(vErr.Error(), tt.errSnippet) {
				t.Errorf("Validate() = %v, want substring %q", vErr, tt.errSnippet)
			}
		})
	}
}

func TestDatabaseSchemaGolden(t *testing.T) {
	checkSchemaGolden(t, "database_schema.golden.json", resource.DatabaseSchema())
}

func checkSchemaGolden(t *testing.T, name string, schema any) {
	t.Helper()
	got, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	goldenPath := filepath.Join("testdata", name)
	if *update {
		if wErr := os.WriteFile(goldenPath, got, 0o600); wErr != nil {
			t.Fatal(wErr)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("%s differs from golden; run `go test ./internal/resource -update`", name)
	}
}
