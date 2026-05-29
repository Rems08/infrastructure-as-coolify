package resource

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateComposePath reports whether rel — a docker_compose_path declared in a Service
// whose YAML lives in baseDir — is safe to read, confined to root. It rejects:
//   - absolute paths,
//   - paths containing NUL or other control characters,
//   - any path that resolves outside root (e.g. "../../../etc/passwd").
//
// A hostile IaC file (a fork PR, a supply-chain payload) must never be able to make
// iac-coolify read a file outside the config tree and ship its contents to Coolify as a
// docker-compose stack. The compose content is read only after this check passes.
func ValidateComposePath(root, baseDir, rel string) error {
	if rel == "" {
		return fmt.Errorf("docker_compose_path: required")
	}
	if i := strings.IndexFunc(rel, isControlRune); i >= 0 {
		return fmt.Errorf("docker_compose_path must not contain control characters (offset %d)", i)
	}
	if filepath.IsAbs(rel) {
		return fmt.Errorf("docker_compose_path must be relative, got absolute path %q", rel)
	}
	abs := filepath.Clean(filepath.Join(baseDir, rel))
	relToRoot, err := filepath.Rel(filepath.Clean(root), abs)
	if err != nil || relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return fmt.Errorf("docker_compose_path must not escape the YAML directory, got %q", rel)
	}
	return nil
}

// isControlRune reports whether r is a NUL, an ASCII control character or DEL — none of
// which belong in a filesystem path and all of which signal an injection attempt.
func isControlRune(r rune) bool {
	return r == 0 || r < 0x20 || r == 0x7f
}
