package plan_test

import (
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
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
