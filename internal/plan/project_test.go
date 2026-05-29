package plan_test

import (
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

func TestProjectorsRoundTrip(t *testing.T) {
	app := resource.Application{
		Metadata: resource.ApplicationMeta{Name: "web"},
		Spec: resource.ApplicationSpec{
			FQDN:  "https://web.example.com",
			Port:  3000,
			Image: &resource.ImageSpec{Name: "registry/web", Tag: "v1"},
		},
	}
	desired := plan.FromApplication(app)
	if desired.Kind != resource.KindApplication || desired.Name != "web" {
		t.Fatalf("projected resource = %+v", desired)
	}
	if len(desired.Fields) != 4 {
		t.Errorf("want 4 fields (fqdn, port, image.name, image.tag), got %d", len(desired.Fields))
	}

	// Same values remotely → no changes.
	remote := plan.FromRemoteApplication(coolify.Application{
		Name:                    "web",
		FQDN:                    "https://web.example.com",
		PortsExposes:            "3000",
		DockerRegistryImageName: "registry/web",
		DockerRegistryImageTag:  "v1",
	})
	if changes := plan.Diff(desired, &remote); len(changes) != 0 {
		t.Errorf("identical projections must not diff: %+v", changes)
	}

	// A drifted remote FQDN must surface.
	remote.Fields[0] = plan.Field{Name: "fqdn", Value: plan.Scalar("https://stale.example.com")}
	changes := plan.Diff(desired, &remote)
	if len(changes) != 1 || changes[0].Op != plan.OpUpdate || changes[0].Path != "Application.web.fqdn" {
		t.Errorf("drift diff = %+v", changes)
	}
}

func TestFromApplicationNoImage(t *testing.T) {
	app := resource.Application{
		Metadata: resource.ApplicationMeta{Name: "svc"},
		Spec:     resource.ApplicationSpec{FQDN: "https://svc", Port: 80},
	}
	desired := plan.FromApplication(app)
	if len(desired.Fields) != 2 {
		t.Errorf("no-image app should project 2 fields, got %d", len(desired.Fields))
	}
}

func beenaireDB() resource.Database {
	return resource.Database{
		Metadata: resource.DatabaseMeta{Name: "pg-staging", Project: "beenaire", Environment: "staging"},
		Spec: resource.DatabaseSpec{
			Engine:      "postgresql",
			Image:       "postgres:18-alpine",
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
		},
	}
}

func remoteBeenaireDB() coolify.Database {
	// Coolify enriches a private database with an internal public_port and default limits;
	// the projection must treat these as stable so a stable database shows no change.
	var pw secrets.Secret
	if err := pw.UnmarshalJSON([]byte(`"runtime-pg-password"`)); err != nil {
		panic(err)
	}
	return coolify.Database{
		Name:             "pg-staging",
		Image:            "postgres:18-alpine",
		IsPublic:         false,
		PublicPort:       5432,
		LimitsCPUShares:  1024,
		LimitsMemory:     "0",
		PostgresPassword: pw,
		InternalDBURL:    pw,
		Status:           "running:healthy",
		CreatedAt:        "2026-01-01T00:00:00Z",
	}
}

func TestPlanDatabase_zeroDiff_onIdenticalConfig(t *testing.T) {
	desired := plan.FromDatabase(beenaireDB())
	remote := plan.FromRemoteDatabase(remoteBeenaireDB())
	if changes := plan.Diff(desired, &remote); len(changes) != 0 {
		t.Errorf("stable database must not diff: %+v", changes)
	}
}

func TestPlanDatabase_updateOnImageChange(t *testing.T) {
	desired := plan.FromDatabase(beenaireDB())
	remoteDB := remoteBeenaireDB()
	remoteDB.Image = "postgres:17-alpine"
	remote := plan.FromRemoteDatabase(remoteDB)
	changes := plan.Diff(desired, &remote)
	if len(changes) != 1 || changes[0].Op != plan.OpUpdate || changes[0].Path != "Database.pg-staging.image" {
		t.Errorf("image drift diff = %+v", changes)
	}
}

func TestPlanDatabase_updateOnMemoryLimitChange(t *testing.T) {
	desiredDB := beenaireDB()
	desiredDB.Spec.Limits = &resource.LimitsSpec{Memory: "512m"}
	desired := plan.FromDatabase(desiredDB)
	remoteDB := remoteBeenaireDB()
	remoteDB.LimitsMemory = "256m"
	remote := plan.FromRemoteDatabase(remoteDB)
	changes := plan.Diff(desired, &remote)
	if len(changes) != 1 || changes[0].Op != plan.OpUpdate || changes[0].Path != "Database.pg-staging.limits.memory" {
		t.Errorf("memory limit drift diff = %+v", changes)
	}
}

func TestPlanDatabase_excludesRuntimeAndSecretFields(t *testing.T) {
	desired := plan.FromDatabase(beenaireDB())
	remoteDB := remoteBeenaireDB()
	// Runtime churn and a rotated credential must not register as drift.
	remoteDB.Status = "restarting"
	remoteDB.RestartCount = 7
	remoteDB.UpdatedAt = "2026-05-29T12:00:00Z"
	var rotated secrets.Secret
	if err := rotated.UnmarshalJSON([]byte(`"a-different-password"`)); err != nil {
		t.Fatal(err)
	}
	remoteDB.PostgresPassword = rotated
	remote := plan.FromRemoteDatabase(remoteDB)
	if changes := plan.Diff(desired, &remote); len(changes) != 0 {
		t.Errorf("runtime/secret churn must not diff: %+v", changes)
	}
}

func TestRenderTextDeleteAndNoop(t *testing.T) {
	var p plan.Plan
	// A noop resource exercises the noop render branch.
	noop := plan.Resource{Kind: "Application", Name: "same", Fields: []plan.Field{{Name: "fqdn", Value: plan.Scalar("https://s")}}}
	p.Add(noop, &noop)
	// An update with a deleted field exercises the delete render branch.
	desired := plan.Resource{Kind: "Application", Name: "web", Fields: []plan.Field{{Name: "fqdn", Value: plan.Scalar("https://new")}}}
	actual := plan.Resource{Kind: "Application", Name: "web", Fields: []plan.Field{
		{Name: "fqdn", Value: plan.Scalar("https://old")},
		{Name: "legacy", Value: plan.Scalar("gone")},
	}}
	p.Add(desired, &actual)

	text := p.RenderText()
	for _, want := range []string{"no changes", "~ Application.web.fqdn", "- Application.web.legacy"} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered text missing %q:\n%s", want, text)
		}
	}
}
