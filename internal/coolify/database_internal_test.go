package coolify

import "testing"

func TestDatabaseCreateEndpoint(t *testing.T) {
	engines := []string{"postgresql", "mysql", "mariadb", "mongodb", "redis", "keydb", "dragonfly", "clickhouse"}
	for _, e := range engines {
		t.Run(e, func(t *testing.T) {
			got, err := databaseCreateEndpoint(e)
			if err != nil {
				t.Fatalf("databaseCreateEndpoint(%q): %v", e, err)
			}
			if want := "/databases/" + e; got != want {
				t.Errorf("endpoint = %q, want %q", got, want)
			}
		})
	}
}

func TestDatabaseCreateEndpointUnknownRejected(t *testing.T) {
	if _, err := databaseCreateEndpoint("cassandra"); err == nil {
		t.Fatal("an unknown engine must be rejected, not mapped to a guessed endpoint")
	}
}
