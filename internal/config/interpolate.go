package config

import (
	"fmt"
	"os"
	"regexp"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
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

// ResolveSecrets binds every deferred ${env:VAR} reference in the desired resources to its
// concrete value, in place, just before apply. Load keeps references unresolved so read-only
// flows stay lenient; this is the single explicit pass — the apply-time companion to the
// lenient loaders — that resolves them. It covers visible (Param) fields, plain env-var
// values and typed env secrets for applications and services, and the password of every
// database. SOPS secrets are already decrypted at load and pass through untouched. A
// referenced env var that is unset yields a clear error naming the resource, so apply fails
// before any push rather than sending an empty value.
func ResolveSecrets(apps []resource.Application, services []LoadedService, dbs []resource.Database) error {
	for i := range apps {
		app := &apps[i]
		if err := interpolateApplicationFields(app); err != nil {
			return fmt.Errorf("application %q: %w", app.Metadata.Name, err)
		}
		if err := interpolateEntries(app.Spec.EnvVars); err != nil {
			return fmt.Errorf("application %q: %w", app.Metadata.Name, err)
		}
		if err := resolveEnvSecretEntries(app.Spec.EnvVars); err != nil {
			return fmt.Errorf("application %q: %w", app.Metadata.Name, err)
		}
	}
	for i := range services {
		svc := &services[i].Service
		if err := interpolateServiceFields(svc); err != nil {
			return fmt.Errorf("service %q: %w", svc.Metadata.Name, err)
		}
		if err := interpolateEntries(svc.Spec.EnvVars); err != nil {
			return fmt.Errorf("service %q: %w", svc.Metadata.Name, err)
		}
		if err := resolveEnvSecretEntries(svc.Spec.EnvVars); err != nil {
			return fmt.Errorf("service %q: %w", svc.Metadata.Name, err)
		}
	}
	for i := range dbs {
		resolved, err := secrets.ResolveEnv(dbs[i].Spec.Password)
		if err != nil {
			return fmt.Errorf("database %q password: %w", dbs[i].Metadata.Name, err)
		}
		dbs[i].Spec.Password = resolved
	}
	return nil
}

// resolveEnvSecretEntries binds every deferred ${env:} typed secret in entries to its value.
// A plain value or an already-resolved/SOPS secret is left untouched.
func resolveEnvSecretEntries(entries []resource.EnvVarEntry) error {
	for i := range entries {
		resolved, err := secrets.ResolveEnv(entries[i].ValueSecret)
		if err != nil {
			return fmt.Errorf("env_vars[%d] %q: %w", i, entries[i].Name, err)
		}
		entries[i].ValueSecret = resolved
	}
	return nil
}

// resolveSecretEntries decrypts any pending ${sops:path} secret in entries, reading the
// secrets.enc.yaml colocated with iacPath. ${env:} secrets are resolved at decode time and
// left untouched here.
func resolveSecretEntries(iacPath string, entries []resource.EnvVarEntry) error {
	for i := range entries {
		if err := resolveSecret(iacPath, &entries[i].ValueSecret); err != nil {
			return fmt.Errorf("env_vars[%d] %q: %w", i, entries[i].Name, err)
		}
	}
	return nil
}

// resolveSecret decrypts a single pending ${sops:path} secret in place; a non-pending
// secret is left unchanged.
func resolveSecret(iacPath string, sec *secrets.Secret) error {
	if !sec.IsPendingSOPS() {
		return nil
	}
	resolved, err := secrets.LoadSOPSValue(iacPath, sec.SOPSPath())
	if err != nil {
		return err
	}
	*sec = resolved
	return nil
}
