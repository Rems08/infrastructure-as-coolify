package config

import (
	"fmt"
	"os"
	"path/filepath"

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
	if err := interpolateApplicationFields(&app); err != nil {
		return app, fmt.Errorf("%s: %w", path, err)
	}
	if err := interpolateEntries(app.Spec.EnvVars); err != nil {
		return app, fmt.Errorf("%s: %w", path, err)
	}
	if err := resolveSecretEntries(path, app.Spec.EnvVars); err != nil {
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
	if err := resolveSecret(path, &db.Spec.Password); err != nil {
		return db, fmt.Errorf("%s: password: %w", path, err)
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
	if err := interpolateStrings(&ev.Metadata.Name, &ev.Metadata.Project, &ev.Metadata.Environment); err != nil {
		return ev, fmt.Errorf("%s: %w", path, err)
	}
	if err := interpolateEntries(ev.Spec.Vars); err != nil {
		return ev, fmt.Errorf("%s: %w", path, err)
	}
	if err := resolveSecretEntries(path, ev.Spec.Vars); err != nil {
		return ev, fmt.Errorf("%s: %w", path, err)
	}
	return ev, nil
}

// LoadProject parses a YAML file into a Project with strict decoding.
func LoadProject(path string) (resource.Project, error) {
	var p resource.Project
	if err := loadStrict(path, &p); err != nil {
		return p, err
	}
	if err := interpolateStrings(&p.Metadata.Name, &p.Spec.Description); err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// LoadEnvironment parses a YAML file into an Environment with strict decoding.
func LoadEnvironment(path string) (resource.Environment, error) {
	var e resource.Environment
	if err := loadStrict(path, &e); err != nil {
		return e, err
	}
	if err := interpolateStrings(&e.Metadata.Name, &e.Metadata.Project, &e.Spec.Description); err != nil {
		return e, fmt.Errorf("%s: %w", path, err)
	}
	return e, nil
}

// LoadService parses a YAML file into a Service with strict decoding, resolving
// ${env:VAR} interpolation in visible env-var values. It does not read the referenced
// compose file; LoadServices does, after the path-traversal check.
func LoadService(path string) (resource.Service, error) {
	var s resource.Service
	if err := loadStrict(path, &s); err != nil {
		return s, err
	}
	if err := interpolateServiceFields(&s); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if err := interpolateEntries(s.Spec.EnvVars); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if err := resolveSecretEntries(path, s.Spec.EnvVars); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// LoadedService pairs a validated Service with its resolved compose content. ComposeRaw is
// the decoded docker-compose file for a compose_path service, or "" for a one-click
// (type) service.
type LoadedService struct {
	Service    resource.Service
	ComposeRaw string
}

// LoadServices loads and validates every Service under target (a file or a directory),
// reading the referenced compose file only after its path passes the traversal check
// (confined to target). Non-Service resources are skipped. It returns the first error
// found.
func LoadServices(target string) ([]LoadedService, error) {
	files, err := collectFiles(target)
	if err != nil {
		return nil, err
	}
	root := composeRoot(target)
	var out []LoadedService
	for _, kf := range files {
		if kf.kind != resource.KindService {
			continue
		}
		svc, lErr := LoadService(kf.path)
		if lErr != nil {
			return nil, lErr
		}
		if vErr := svc.Validate(); vErr != nil {
			return nil, fmt.Errorf("%s: %w", kf.path, vErr)
		}
		raw, cErr := resolveCompose(root, kf.path, svc)
		if cErr != nil {
			return nil, cErr
		}
		out = append(out, LoadedService{Service: svc, ComposeRaw: raw})
	}
	return out, nil
}

// composeRoot returns the directory that bounds docker_compose_path resolution: the target
// directory itself, or the directory holding a single target file.
func composeRoot(target string) string {
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return target
	}
	return filepath.Dir(target)
}

// resolveCompose validates a compose_path against root and reads its content. It returns
// "" for a one-click (type) service, which has no local file.
func resolveCompose(root, path string, svc resource.Service) (string, error) {
	if !svc.Spec.HasComposePath() {
		return "", nil
	}
	baseDir := filepath.Dir(path)
	rel := svc.Spec.DockerComposePath
	if err := resource.ValidateComposePath(root, baseDir, rel); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	data, err := os.ReadFile(filepath.Join(baseDir, rel))
	if err != nil {
		return "", fmt.Errorf("%s: read compose file: %w", path, err)
	}
	return string(data), nil
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

// LoadProjects loads and validates every Project under target (a file or directory).
// Non-Project resources are skipped. It returns the first error found.
func LoadProjects(target string) ([]resource.Project, error) {
	files, err := collectFiles(target)
	if err != nil {
		return nil, err
	}
	var projects []resource.Project
	for _, kf := range files {
		if kf.kind != resource.KindProject {
			continue
		}
		p, lErr := LoadProject(kf.path)
		if lErr != nil {
			return nil, lErr
		}
		if vErr := p.Validate(); vErr != nil {
			return nil, fmt.Errorf("%s: %w", kf.path, vErr)
		}
		projects = append(projects, p)
	}
	return projects, nil
}

// LoadEnvironments loads and validates every Environment under target (a file or
// directory). Non-Environment resources are skipped. It returns the first error found.
func LoadEnvironments(target string) ([]resource.Environment, error) {
	files, err := collectFiles(target)
	if err != nil {
		return nil, err
	}
	var envs []resource.Environment
	for _, kf := range files {
		if kf.kind != resource.KindEnvironment {
			continue
		}
		e, lErr := LoadEnvironment(kf.path)
		if lErr != nil {
			return nil, lErr
		}
		if vErr := e.Validate(); vErr != nil {
			return nil, fmt.Errorf("%s: %w", kf.path, vErr)
		}
		envs = append(envs, e)
	}
	return envs, nil
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
