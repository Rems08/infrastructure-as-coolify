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
	fields = append(fields, destinationFields(app.Spec.Destination.Server, app.Spec.Destination.Network)...)
	return Resource{Kind: resource.KindApplication, Name: app.Metadata.Name, Fields: fields}
}

// FromRemoteApplication projects a live Coolify application into a diffable Resource,
// mirroring the field set of FromApplication. The destination projects the server NAME
// (never its UUID): the desired YAML carries logical names, so comparing UUIDs would
// report a phantom move on every plan.
func FromRemoteApplication(app coolify.Application) Resource {
	fields := []Field{
		{Name: "fqdn", Value: Scalar(app.FQDN)},
		{Name: "port", Value: Scalar(app.PortsExposes)},
		{Name: "image.name", Value: Scalar(app.DockerRegistryImageName)},
		{Name: "image.tag", Value: Scalar(app.DockerRegistryImageTag)},
	}
	fields = append(fields, destinationFields(app.Destination.Server.Name, app.Destination.Network)...)
	return Resource{Kind: resource.KindApplication, Name: app.Name, Fields: fields}
}

// destinationFields projects the destination pair shared by applications and databases.
// Both fields force a recreate: the Coolify API has no in-place server move, so a PATCH
// could never converge a destination change.
func destinationFields(server, network string) []Field {
	return []Field{
		{Name: "destination.server", Value: Scalar(server), ForcesRecreate: true},
		{Name: "destination.network", Value: Scalar(network), ForcesRecreate: true},
	}
}

// coolifyDefaultCPUShares is the value Coolify assigns when no CPU-share limit is set. It
// is treated as "unset" when projecting, so an undeclared limit does not surface as drift.
const coolifyDefaultCPUShares = 1024

// FromDatabase projects a desired Database config into a diffable Resource.
//
// The password is intentionally excluded: a secret is only ever [REDACTED] on both sides,
// so field-diffing it could not show anything actionable without risking a leak; a
// credential rotation is an explicit operation, not a drift the plan reconciles. Runtime
// fields (status, timestamps, restart count, the credential-bearing connection URLs) are
// excluded for the same reasons FromRemoteDatabase omits them. public_port and limits are
// compared only when meaningfully set (see FromRemoteDatabase).
func FromDatabase(d resource.Database) Resource {
	fields := []Field{
		{Name: "image", Value: Scalar(d.Spec.Image)},
		{Name: "public", Value: Scalar(strconv.FormatBool(d.Spec.Public))},
	}
	if d.Spec.Public {
		fields = append(fields, Field{Name: "public_port", Value: Scalar(strconv.Itoa(d.Spec.PublicPort))})
	}
	fields = append(fields, destinationFields(d.Spec.Destination.Server, d.Spec.Destination.Network)...)
	if d.Spec.Limits != nil {
		fields = appendCPUShares(fields, d.Spec.Limits.CPUShares, 0)
		fields = appendMemory(fields, d.Spec.Limits.Memory)
	}
	return Resource{Kind: resource.KindDatabase, Name: d.Metadata.Name, Fields: fields}
}

// FromRemoteDatabase projects a live Coolify database into a diffable Resource, mirroring
// the field set of FromDatabase. public_port is compared only when the database is public:
// Coolify assigns an internal port even to a private database, and comparing that
// runtime-assigned port against an undeclared one would report a phantom change.
func FromRemoteDatabase(d coolify.Database) Resource {
	fields := []Field{
		{Name: "image", Value: Scalar(d.Image)},
		{Name: "public", Value: Scalar(strconv.FormatBool(d.IsPublic))},
	}
	if d.IsPublic {
		fields = append(fields, Field{Name: "public_port", Value: Scalar(strconv.Itoa(d.PublicPort))})
	}
	fields = append(fields, destinationFields(d.Destination.Server.Name, d.Destination.Network)...)
	fields = appendCPUShares(fields, d.LimitsCPUShares, coolifyDefaultCPUShares)
	fields = appendMemory(fields, d.LimitsMemory)
	return Resource{Kind: resource.KindDatabase, Name: d.Name, Fields: fields}
}

// appendCPUShares adds the limits.cpu_shares field unless shares is zero or equals unset
// (the engine default), which both mean "no limit declared".
func appendCPUShares(fields []Field, shares, unset int) []Field {
	if shares == 0 || shares == unset {
		return fields
	}
	return append(fields, Field{Name: "limits.cpu_shares", Value: Scalar(strconv.Itoa(shares))})
}

// appendMemory adds the limits.memory field unless it is empty or "0" (no limit declared).
func appendMemory(fields []Field, mem string) []Field {
	if mem == "" || mem == "0" {
		return fields
	}
	return append(fields, Field{Name: "limits.memory", Value: Scalar(mem)})
}
