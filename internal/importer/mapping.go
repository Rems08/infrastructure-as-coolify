// Package importer reverse-engineers a live Coolify instance into local iac-coolify YAML
// manifests. It enumerates resources per server (the only path that reveals which server
// hosts each resource), maps the live application and database shapes back to the
// user-facing resource types, and writes them through the leak-safe config marshallers.
//
// Two remote shapes cannot be fully reconstructed and are handled explicitly: a git-based
// application's repository URL is not exposed by the API (a sentinel placeholder is written
// and the application is reported as partial), and an environment variable's value is
// returned in clear text (it is never written; a ${env:KEY} reference is emitted instead so
// the operator populates a .env). Services are not imported at all: the API exposes only a
// name and environment for them, not enough to rebuild a spec.
package importer

import (
	"fmt"
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
)

// gitRepositoryPlaceholder is written for an application built from a git source: the
// Coolify v4 API does not expose the repository URL, so the field cannot be reconstructed.
// It is deliberately not a valid URL so `validate` flags it, prompting a manual edit.
const gitRepositoryPlaceholder = "TODO-not-exposed-by-coolify-api"

// envRef pairs an environment name with its owning project, derived from enumerating each
// project's environments (never from a payload's project_id, which the API leaves zero).
type envRef struct {
	project string
	name    string
}

// mapApplication builds a resource.Application from the live application detail, the server
// it was found on, the network supplied by the caller (the API does not expose it), and its
// environment. Each env var becomes a ${env:KEY} reference rather than its clear value.
// It returns the env-var keys referenced, so the caller can tell the operator which to set.
func mapApplication(app coolify.Application, server, network string, env envRef, envs []coolify.ServiceEnvVar) (resource.Application, []string) {
	hasImage := app.DockerRegistryImageName != ""
	spec := resource.ApplicationSpec{
		BuildPack:   mapBuildPack(app.BuildPack, hasImage),
		Destination: resource.DestinationRef{Server: server, Network: network},
		FQDN:        app.FQDN,
	}
	if hasImage {
		spec.Image = &resource.ImageSpec{Name: app.DockerRegistryImageName, Tag: app.DockerRegistryImageTag}
		spec.Port = firstPort(app.PortsExposes)
	} else {
		spec.Source = &resource.SourceSpec{
			GitRepository: gitRepositoryPlaceholder,
			GitBranch:     app.GitBranch,
			PortsExposes:  app.PortsExposes,
		}
	}
	keys := make([]string, 0, len(envs))
	for _, e := range envs {
		spec.EnvVars = append(spec.EnvVars, resource.EnvVarEntry{Name: e.Key, Value: envReference(e.Key)})
		keys = append(keys, e.Key)
	}
	return resource.Application{
		APIVersion: resource.APIVersion,
		Kind:       resource.KindApplication,
		Metadata:   resource.ApplicationMeta{Name: app.Name, Project: env.project, Environment: env.name},
		Spec:       spec,
	}, keys
}

// mapDatabase builds a resource.Database from the live database detail, the server it was
// found on, the network, and its environment. The live password is an opaque remote secret
// with no source declaration that the marshaller would refuse; in its place a synthetic
// ${env:<NAME>_PASSWORD} reference is emitted, and its env-var name returned for the report.
func mapDatabase(db coolify.Database, server, network string, env envRef) (resource.Database, string, error) {
	engine, err := mapEngine(db.DatabaseType)
	if err != nil {
		return resource.Database{}, "", err
	}
	passwordEnv := sanitizeEnvName(db.Name) + "_PASSWORD"
	password, err := secrets.NewReference(envReference(passwordEnv))
	if err != nil {
		return resource.Database{}, "", err
	}
	spec := resource.DatabaseSpec{
		Engine:      engine,
		Image:       db.Image,
		Destination: resource.DestinationRef{Server: server, Network: network},
		Public:      db.IsPublic,
		Password:    password,
	}
	if db.IsPublic {
		spec.PublicPort = db.PublicPort
	}
	return resource.Database{
		APIVersion: resource.APIVersion,
		Kind:       resource.KindDatabase,
		Metadata:   resource.DatabaseMeta{Name: db.Name, Project: env.project, Environment: env.name},
		Spec:       spec,
	}, passwordEnv, nil
}

// mapBuildPack maps a live Coolify build_pack to the iac-coolify enum. An application that
// carries a docker registry image is a dockerimage build regardless of the upstream value;
// otherwise the upstream spelling is translated (dockercompose → docker-compose) or passed
// through. A value outside the iac enum is returned as-is so the written manifest fails
// `validate` loudly rather than silently mislabelling the build.
func mapBuildPack(remote string, hasImage bool) string {
	if hasImage {
		return "dockerimage"
	}
	if remote == "dockercompose" {
		return "docker-compose"
	}
	return remote
}

// mapEngine maps a live database_type (e.g. "standalone-postgresql") to the iac-coolify
// engine enum by stripping the "standalone-" prefix the runtime API uses.
func mapEngine(databaseType string) (string, error) {
	engine := strings.TrimPrefix(databaseType, "standalone-")
	if engine == "" {
		return "", fmt.Errorf("importer: empty database_type")
	}
	return engine, nil
}

// firstPort parses the first port from a Coolify ports_exposes string ("3000" or
// "3000,8080"), returning 0 when it holds no parsable port. A multi-port value keeps only
// the first; the others are dropped (a dockerimage application declares a single port).
func firstPort(portsExposes string) int {
	for _, part := range strings.Split(portsExposes, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n := 0
		if _, err := fmt.Sscanf(part, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// envReference renders the deferred reference written in place of a clear value.
func envReference(key string) string { return "${env:" + key + "}" }

// sanitizeEnvName upper-cases name and replaces every character that is not a letter or
// digit with an underscore, yielding a valid shell env-var stem (e.g.
// "pg-restaurant-api-staging" → "PG_RESTAURANT_API_STAGING").
func sanitizeEnvName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - ('a' - 'A'))
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
