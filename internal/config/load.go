package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// PeekKind reads only the `kind` discriminator from a YAML file, tolerating any other
// fields. It is used to select Application files when walking a directory.
func PeekKind(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	var head struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(data, &head); err != nil {
		return "", fmt.Errorf("parse %s: %w", path, err)
	}
	return head.Kind, nil
}

// LoadApplication parses a YAML file into an Application with strict decoding (unknown
// and duplicate fields are rejected — mitigation C-S2.6, threat-model T-S2.6) and
// resolves ${env:VAR} interpolation in visible env-var values.
func LoadApplication(path string) (resource.Application, error) {
	var app resource.Application
	data, err := os.ReadFile(path)
	if err != nil {
		return app, fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.UnmarshalWithOptions(data, &app, yaml.Strict()); err != nil {
		return app, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := interpolateEnvVars(&app); err != nil {
		return app, fmt.Errorf("%s: %w", path, err)
	}
	return app, nil
}

// interpolateEnvVars resolves ${env:VAR} in each visible env-var value. Secret values
// (value_secret) are resolved by secrets.Secret at decode time and are skipped here.
func interpolateEnvVars(app *resource.Application) error {
	for i := range app.Spec.EnvVars {
		ev := &app.Spec.EnvVars[i]
		if ev.Value == "" {
			continue
		}
		resolved, err := ResolveEnvInterpolation(ev.Value)
		if err != nil {
			return fmt.Errorf("env_vars[%d] %q: %w", i, ev.Name, err)
		}
		ev.Value = resolved
	}
	return nil
}
