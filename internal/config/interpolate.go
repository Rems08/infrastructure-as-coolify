package config

import (
	"fmt"
	"os"
	"regexp"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// envPattern matches ${env:VAR} where VAR is an upper-snake identifier. The anchor
// to [A-Z_][A-Z0-9_]* means lowercase names or names with spaces never match.
var envPattern = regexp.MustCompile(`\$\{env:([A-Z_][A-Z0-9_]*)\}`)

// ResolveEnvInterpolation replaces every ${env:VAR} occurrence in raw with
// os.Getenv(VAR). It returns an error if any referenced env var is unset — there
// is no silent fallback to "". Used by the YAML parser for non-Secret string fields
// (Params); Secret fields go through secrets.Secret.UnmarshalYAML instead.
func ResolveEnvInterpolation(raw string) (string, error) {
	var firstErr error
	out := envPattern.ReplaceAllStringFunc(raw, func(match string) string {
		name := envPattern.FindStringSubmatch(match)[1]
		v, ok := os.LookupEnv(name)
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("env var %q referenced but not set", name)
			}
			return match
		}
		return v
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

// interpolateStrings resolves ${env:VAR} in each non-empty target in place. It is the
// generic Param-field interpolation used by every resource loader; typed Secret fields are
// never passed here (they carry their own ${env:}/${sops:} resolution).
func interpolateStrings(targets ...*string) error {
	for _, p := range targets {
		if *p == "" {
			continue
		}
		resolved, err := ResolveEnvInterpolation(*p)
		if err != nil {
			return err
		}
		*p = resolved
	}
	return nil
}

// interpolateApplicationFields resolves ${env:VAR} in every visible (Param) string field of
// an application. Typed Secret env-var values resolve their own ${env:} at decode time and
// are never touched here.
func interpolateApplicationFields(app *resource.Application) error {
	targets := []*string{
		&app.Metadata.Name, &app.Metadata.Project, &app.Metadata.Environment,
		&app.Spec.FQDN, &app.Spec.Dockerfile,
		&app.Spec.Destination.Server, &app.Spec.Destination.Network,
	}
	if app.Spec.Image != nil {
		targets = append(targets, &app.Spec.Image.Name, &app.Spec.Image.Tag)
	}
	if app.Spec.Source != nil {
		targets = append(targets, &app.Spec.Source.GitRepository, &app.Spec.Source.GitBranch, &app.Spec.Source.PortsExposes)
	}
	if app.Spec.Limits != nil {
		targets = append(targets, &app.Spec.Limits.Memory)
	}
	if app.Spec.HealthCheck != nil {
		targets = append(targets, &app.Spec.HealthCheck.Path)
	}
	return interpolateStrings(targets...)
}

// interpolateServiceFields resolves ${env:VAR} in every visible string field of a service.
func interpolateServiceFields(svc *resource.Service) error {
	return interpolateStrings(
		&svc.Metadata.Name, &svc.Metadata.Project, &svc.Metadata.Environment,
		&svc.Spec.Description, &svc.Spec.FQDN, &svc.Spec.Type, &svc.Spec.DockerComposePath,
		&svc.Spec.Destination.Server,
	)
}
