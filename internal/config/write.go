package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// WriteApplication serialises app back to a YAML manifest at path, preserving the full
// document (api_version, kind, metadata, spec). A secret is emitted as its source
// declaration (${env:NAME} / ${sops:path}) by secrets.Secret.MarshalYAML, never as a
// resolved value; a secret that carries no such declaration is refused before any write
// (see guardWritableSecrets). The file is written atomically: a sibling temp file is
// renamed into place, so a crash mid-write cannot leave a truncated manifest.
func WriteApplication(path string, app resource.Application) error {
	if err := guardWritableSecrets(app); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	data, err := yaml.Marshal(app)
	if err != nil {
		return fmt.Errorf("marshal application %q: %w", app.Metadata.Name, err)
	}
	return atomicWrite(path, data)
}

// guardWritableSecrets refuses to serialise a secret with no source declaration. A secret
// sourced from ${env:} or ${sops:} carries an origin that re-emits safely; a secret read
// back from the API (or otherwise forged) has none, and writing it would either drop the
// reference or risk leaking the live value — neither is acceptable, so it is an error.
func guardWritableSecrets(app resource.Application) error {
	for i := range app.Spec.EnvVars {
		e := app.Spec.EnvVars[i]
		if !e.ValueSecret.IsZero() && e.ValueSecret.Origin() == "" {
			return fmt.Errorf(
				"env_var %q: cannot serialise a secret with no ${env:}/${sops:} declaration",
				e.Name,
			)
		}
	}
	return nil
}

// atomicWrite writes data to path via a temp-file-then-rename, so a reader never observes a
// partial file. A new file is created 0600; overwriting an existing manifest preserves its
// current mode so a write-back does not silently widen or narrow its permissions.
func atomicWrite(path string, data []byte) error {
	perm := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".iac-coolify-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file to %s: %w", path, err)
	}
	return nil
}
