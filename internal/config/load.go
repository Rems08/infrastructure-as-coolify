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
// and duplicate fields are rejected) and resolves ${env:VAR} interpolation in visible
// env-var values.
func LoadApplication(path string) (resource.Application, error) {
	var app resource.Application
	if err := loadStrict(path, &app); err != nil {
		return app, err
	}
	if err := interpolateEntries(app.Spec.EnvVars); err != nil {
		return app, fmt.Errorf("%s: %w", path, err)
	}
	return app, nil
}

// LoadDatabase parses a YAML file into a Database with strict decoding.
func LoadDatabase(path string) (resource.Database, error) {
	var db resource.Database
	if err := loadStrict(path, &db); err != nil {
		return db, err
	}
	return db, nil
}

// LoadEnvVar parses a YAML file into a standalone EnvVar resource with strict decoding,
// resolving ${env:VAR} interpolation in visible values.
func LoadEnvVar(path string) (resource.EnvVar, error) {
	var ev resource.EnvVar
	if err := loadStrict(path, &ev); err != nil {
		return ev, err
	}
	if err := interpolateEntries(ev.Spec.Vars); err != nil {
		return ev, fmt.Errorf("%s: %w", path, err)
	}
	return ev, nil
}

// LoadApplications loads and validates every Application under target (a file or a
// directory). Non-Application resources are skipped. It returns the first error found.
func LoadApplications(target string) ([]resource.Application, error) {
	files, err := collectFiles(target)
	if err != nil {
		return nil, err
	}
	var apps []resource.Application
	for _, kf := range files {
		if kf.kind != resource.KindApplication {
			continue
		}
		app, lErr := LoadApplication(kf.path)
		if lErr != nil {
			return nil, lErr
		}
		if vErr := app.Validate(); vErr != nil {
			return nil, fmt.Errorf("%s: %w", kf.path, vErr)
		}
		apps = append(apps, app)
	}
	return apps, nil
}

// loadStrict reads path and strictly decodes it into dst (unknown/duplicate fields are
// rejected).
func loadStrict(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.UnmarshalWithOptions(data, dst, yaml.Strict()); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// interpolateEntries resolves ${env:VAR} in each visible value. Secret values
// (value_secret) are resolved by secrets.Secret at decode time and are skipped here.
func interpolateEntries(entries []resource.EnvVarEntry) error {
	for i := range entries {
		ev := &entries[i]
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
