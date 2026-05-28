package plan

import (
	"strconv"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// FromApplication projects a desired Application config into a diffable Resource.
//
// Only the core fields that need no IaC→API translation are compared here. build_pack
// (enum mapping) and env-var reconciliation are intentionally absent from the diff
// projection — the diff engine itself already supports secret fields (see diff_test.go),
// they are wired into the projection once envs are fetched by an apply command.
func FromApplication(app resource.Application) Resource {
	fields := []Field{
		{Name: "fqdn", Value: Scalar(app.Spec.FQDN)},
		{Name: "port", Value: Scalar(strconv.Itoa(app.Spec.Port))},
	}
	if app.Spec.Image != nil {
		fields = append(fields,
			Field{Name: "image.name", Value: Scalar(app.Spec.Image.Name)},
			Field{Name: "image.tag", Value: Scalar(app.Spec.Image.Tag)},
		)
	}
	return Resource{Kind: resource.KindApplication, Name: app.Metadata.Name, Fields: fields}
}

// FromRemoteApplication projects a live Coolify application into a diffable Resource,
// mirroring the field set of FromApplication.
func FromRemoteApplication(app coolify.Application) Resource {
	return Resource{
		Kind: resource.KindApplication,
		Name: app.Name,
		Fields: []Field{
			{Name: "fqdn", Value: Scalar(app.FQDN)},
			{Name: "port", Value: Scalar(app.PortsExposes)},
			{Name: "image.name", Value: Scalar(app.DockerRegistryImageName)},
			{Name: "image.tag", Value: Scalar(app.DockerRegistryImageTag)},
		},
	}
}
