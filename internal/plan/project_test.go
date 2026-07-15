package plan_test

import (
	"encoding/json"
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
			FQDN:        "https://web.example.com",
			Port:        3000,
			Image:       &resource.ImageSpec{Name: "registry/web", Tag: "v1"},
			Destination: resource.DestinationRef{Server: "localhost", Network: "coolify"},
		},
	}
	desired := plan.FromApplication(app)
	if desired.Kind != resource.KindApplication || desired.Name != "web" {
		t.Fatalf("projected resource = %+v", desired)
	}
	if len(desired.Fields) != 7 {
		t.Errorf("want 7 fields (description, fqdn, port, image.name, image.tag, destination.server, destination.network), got %d", len(desired.Fields))
	}

	// Same values remotely → no changes.
	remote := plan.FromRemoteApplication(coolify.Application{
		Name:                    "web",
		FQDN:                    "https://web.example.com",
		PortsExposes:            "3000",
		DockerRegistryImageName: "registry/web",
		DockerRegistryImageTag:  "v1",
		Destination:             webDestination(),
	})
	if changes := plan.Diff(desired, &remote); len(changes) != 0 {
		t.Errorf("identical projections must not diff: %+v", changes)
	}

	// A drifted remote FQDN must surface.
	remote.Fields[1] = plan.Field{Name: "fqdn", Value: plan.Scalar("https://stale.example.com")}
	changes := plan.Diff(desired, &remote)
	if len(changes) != 1 || changes[0].Op != plan.OpUpdate || changes[0].Path != "Application.web.fqdn" {
		t.Errorf("drift diff = %+v", changes)
	}

	// A drifted description must surface as an ordinary update, not a recreate.
	remote.Fields[1] = plan.Field{Name: "fqdn", Value: plan.Scalar("https://web.example.com")}
	remote.Fields[0] = plan.Field{Name: "description", Value: plan.Scalar("stale role")}
	changes = plan.Diff(desired, &remote)
	if len(changes) != 1 || changes[0].Path != "Application.web.description" || changes[0].RequiresRecreate {
		t.Errorf("description drift diff = %+v", changes)
	}
}

func TestFromApplicationNoImage(t *testing.T) {
	app := resource.Application{
		Metadata: resource.ApplicationMeta{Name: "svc"},
		Spec:     resource.ApplicationSpec{FQDN: "https://svc", Port: 80},
	}
	desired := plan.FromApplication(app)
	if len(desired.Fields) != 5 {
		t.Errorf("no-image app should project 5 fields (description, fqdn, port, destination pair), got %d", len(desired.Fields))
	}
}

// webDestination is the live destination shape matching the "web" fixture: server
// localhost on the coolify network.
func webDestination() coolify.Destination {
	return coolify.Destination{
		Name:    "coolify",
		Network: "coolify",
		Server:  coolify.Server{UUID: "srv-uuid", Name: "localhost"},
	}
}

func webApp(server, network string) resource.Application {
	return resource.Application{
		Metadata: resource.ApplicationMeta{Name: "web"},
		Spec: resource.ApplicationSpec{
			FQDN:        "https://web.example.com",
			Port:        3000,
			Image:       &resource.ImageSpec{Name: "registry/web", Tag: "v1"},
			Destination: resource.DestinationRef{Server: server, Network: network},
		},
	}
}

func remoteWebApp() coolify.Application {
	return coolify.Application{
		Name:                    "web",
		FQDN:                    "https://web.example.com",
		PortsExposes:            "3000",
		DockerRegistryImageName: "registry/web",
		DockerRegistryImageTag:  "v1",
		Destination:             webDestination(),
	}
}

func TestDestinationServerChange_RequiresRecreate(t *testing.T) {
	desired := plan.FromApplication(webApp("hetzner-1", "coolify"))
	remote := plan.FromRemoteApplication(remoteWebApp())
	changes := plan.Diff(desired, &remote)
	if len(changes) != 1 {
		t.Fatalf("want 1 change, got %+v", changes)
	}
	c := changes[0]
	if c.Path != "Application.web.destination.server" || c.Op != plan.OpUpdate {
		t.Errorf("change = %+v", c)
	}
	if !c.RequiresRecreate {
		t.Error("a destination.server change must require recreate")
	}
	if c.Old != "localhost" || c.New != "hetzner-1" {
		t.Errorf("server change = %q -> %q, want logical names (never uuids)", c.Old, c.New)
	}
}

func TestDestinationNetworkChange_RequiresRecreate(t *testing.T) {
	desired := plan.FromApplication(webApp("localhost", "other-net"))
	remote := plan.FromRemoteApplication(remoteWebApp())
	changes := plan.Diff(desired, &remote)
	if len(changes) != 1 || changes[0].Path != "Application.web.destination.network" || !changes[0].RequiresRecreate {
		t.Errorf("network change diff = %+v", changes)
	}
}

// TestDestinationUnresolvedEnv_NoDiff guards the plan-time behaviour for parameterised
// manifests: a destination still carrying a ${env:} reference is unknown until apply, so it
// must not surface as a phantom recreate against the live server name.
func TestDestinationUnresolvedEnv_NoDiff(t *testing.T) {
	desired := plan.FromApplication(webApp("${env:STAGING_SERVER}", "coolify"))
	remote := plan.FromRemoteApplication(remoteWebApp())
	if changes := plan.Diff(desired, &remote); len(changes) != 0 {
		t.Errorf("unresolved env destination must not diff, got %+v", changes)
	}
}

func TestDestinationIdentical_Noop(t *testing.T) {
	desired := plan.FromApplication(webApp("localhost", "coolify"))
	remote := plan.FromRemoteApplication(remoteWebApp())
	if changes := plan.Diff(desired, &remote); len(changes) != 0 {
		t.Errorf("identical destination must not diff, got %+v", changes)
	}
}

// TestOrdinaryUpdate_NoRecreateMarker asserts a patchable field change (the image tag) stays
// a plain update: neither the change nor the item carries the recreate marker.
func TestOrdinaryUpdate_NoRecreateMarker(t *testing.T) {
	app := webApp("localhost", "coolify")
	app.Spec.Image = &resource.ImageSpec{Name: "registry/web", Tag: "v2"}
	remoteApp := remoteWebApp()
	remoteApp.DockerRegistryImageName = "registry/web"
	remoteApp.DockerRegistryImageTag = "v1"
	remote := plan.FromRemoteApplication(remoteApp)

	var p plan.Plan
	p.Add(plan.FromApplication(app), &remote)
	if len(p.Items) != 1 {
		t.Fatalf("items = %+v", p.Items)
	}
	it := p.Items[0]
	if it.Action != plan.ActionUpdate || it.RequiresRecreate {
		t.Errorf("item = %+v, want a plain update without recreate", it)
	}
	for _, c := range it.Changes {
		if c.RequiresRecreate {
			t.Errorf("change %+v must not require recreate", c)
		}
	}
}

func TestPlanJSONAndText_MarkRecreate(t *testing.T) {
	var p plan.Plan
	remote := plan.FromRemoteApplication(remoteWebApp())
	p.Add(plan.FromApplication(webApp("hetzner-1", "coolify")), &remote)

	it := p.Items[0]
	if it.Action != plan.ActionUpdate || !it.RequiresRecreate {
		t.Fatalf("item = %+v, want update + requires recreate", it)
	}
	blob, err := json.Marshal(p.Output())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(blob), `"requires_recreate":true`) {
		t.Errorf("json output missing requires_recreate marker:\n%s", blob)
	}

	text := p.RenderText()
	want := `-/+ Application.web must be recreated (destination changed: server "localhost" -> "hetzner-1")`
	if !strings.Contains(text, want) {
		t.Errorf("text output missing %q:\n%s", want, text)
	}
	// The summary keeps the resource in the change count: no fourth counter.
	if !strings.Contains(text, "Plan: 0 to add, 1 to change, 0 to destroy.") {
		t.Errorf("summary mismatch:\n%s", text)
	}
}

func TestPlanDatabase_destinationChange_RequiresRecreate(t *testing.T) {
	desiredDB := beenaireDB()
	desiredDB.Spec.Destination.Server = "hetzner-1"
	desired := plan.FromDatabase(desiredDB)
	remote := plan.FromRemoteDatabase(remoteBeenaireDB())
	changes := plan.Diff(desired, &remote)
	if len(changes) != 1 || changes[0].Path != "Database.pg-staging.destination.server" || !changes[0].RequiresRecreate {
		t.Errorf("database destination diff = %+v", changes)
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
		Destination:      webDestination(),
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
