package coolify

// Application mirrors the MVP subset of the Coolify v4 `Application` schema
// (testdata/openapi/coolify-v4.yaml, components.schemas.Application). Only the
// fields iac-coolify reads in Wave 1 are modelled; the full schema has ~90 fields.
//
// NOTE: build_pack here reflects the upstream OpenAPI enum
// (nixpacks|railpack|static|dockerfile|dockercompose) and is intentionally a plain
// string for forward-compat. The user-facing IaC enum lives in internal/resource and
// differs (see G-W1-build_pack-enum-mismatch); the IaC→API mapping arrives in Wave 2.
type Application struct {
	UUID                    string `json:"uuid"`
	Name                    string `json:"name"`
	FQDN                    string `json:"fqdn"`
	BuildPack               string `json:"build_pack"`
	DockerRegistryImageName string `json:"docker_registry_image_name"`
	DockerRegistryImageTag  string `json:"docker_registry_image_tag"`
	GitBranch               string `json:"git_branch"`
	PortsExposes            string `json:"ports_exposes"`
	Status                  string `json:"status"`
}
