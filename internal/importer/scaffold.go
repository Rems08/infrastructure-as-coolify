package importer

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// requiredCoolify is the Coolify version constraint written into the scaffolded root
// manifest, matching the constraint the bundled examples declare.
const requiredCoolify = ">=4.0.0,<5.0.0"

// ErrManifestsExist is returned by Run when one or more target manifests already exist and
// Force is false. Callers test for it (errors.Is) to offer an overwrite rather than parsing
// the message.
var ErrManifestsExist = errors.New("manifests already exist")

// plannedFile is one manifest the import will write: its absolute target path and the
// closure that serialises it. Collecting the closures lets the import detect every conflict
// before writing anything, so a refused import never leaves a half-written tree.
type plannedFile struct {
	path  string
	write func() error
}

// applicationPath returns the manifest path for an application under dir, following the
// layout the loaders expect: environments/<env>/applications/<name>.yaml.
func applicationPath(dir, env, name string) string {
	return filepath.Join(dir, "environments", env, "applications", name+".yaml")
}

// databasePath returns the manifest path for a database under dir
// (environments/<env>/databases/<name>.yaml).
func databasePath(dir, env, name string) string {
	return filepath.Join(dir, "environments", env, "databases", name+".yaml")
}

// rootManifestPath returns the path of the scaffolded project root manifest.
func rootManifestPath(dir string) string { return filepath.Join(dir, "coolify.yaml") }

// planApplication returns the planned write for an application manifest.
func planApplication(dir string, app resource.Application) plannedFile {
	path := applicationPath(dir, app.Metadata.Environment, app.Metadata.Name)
	return plannedFile{path: path, write: func() error { return config.WriteApplication(path, app) }}
}

// planDatabase returns the planned write for a database manifest.
func planDatabase(dir string, db resource.Database) plannedFile {
	path := databasePath(dir, db.Metadata.Environment, db.Metadata.Name)
	return plannedFile{path: path, write: func() error { return config.WriteDatabase(path, db) }}
}

// planRoot returns the planned write for the project root manifest (coolify.yaml).
func planRoot(dir, apiURL string) plannedFile {
	path := rootManifestPath(dir)
	return plannedFile{path: path, write: func() error { return writeRootManifest(path, apiURL) }}
}

// writeRootManifest writes the project root coolify.yaml carrying the schema version, the
// supported Coolify range, and the instance URL the import read from.
func writeRootManifest(path, apiURL string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "api_version: %s\n", resource.APIVersion)
	fmt.Fprintf(&b, "required_coolify: %q\n", requiredCoolify)
	if apiURL != "" {
		fmt.Fprintf(&b, "api_url: %s\n", apiURL)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// ScaffoldRoot writes the project root coolify.yaml under dir if it is absent, recording
// apiURL, and reports whether it wrote. An existing root is left untouched (wrote is false),
// so re-running over a populated directory is a no-op rather than an overwrite.
func ScaffoldRoot(dir, apiURL string) (bool, error) {
	path := rootManifestPath(dir)
	switch _, err := os.Stat(path); {
	case err == nil:
		return false, nil
	case !errors.Is(err, fs.ErrNotExist):
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	if err := writeRootManifest(path, apiURL); err != nil {
		return false, err
	}
	return true, nil
}

// commitFiles writes every planned file, unless one already exists and force is false: in
// that case it refuses with an error listing all collisions and writes nothing, so an import
// is all-or-nothing. With force, existing files are overwritten. Parent directories are
// created as needed.
func commitFiles(files []plannedFile, force bool) error {
	if !force {
		if collisions := existingPaths(files); len(collisions) > 0 {
			return fmt.Errorf(
				"%w: refusing to overwrite %d existing file(s) (use --force to overwrite):\n  %s",
				ErrManifestsExist, len(collisions), strings.Join(collisions, "\n  "),
			)
		}
	}
	for _, f := range files {
		if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(f.path), err)
		}
		if err := f.write(); err != nil {
			return err
		}
	}
	return nil
}

// existingPaths returns the sorted target paths that already exist on disk.
func existingPaths(files []plannedFile) []string {
	var out []string
	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			out = append(out, f.path)
		}
	}
	sort.Strings(out)
	return out
}
