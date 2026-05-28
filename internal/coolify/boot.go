package coolify

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// specFileName and sidecarFileName are the pinned OpenAPI artefacts under the openapi dir.
const (
	specFileName    = "coolify-v4.yaml"
	sidecarFileName = "coolify-v4.yaml.sha256"
)

// ErrSpecAbsent signals the pinned OpenAPI artefacts are not on disk (e.g. the binary
// runs outside the repository). Absence is not tampering: callers may skip boot
// verification and rely on the compiled-in openAPIChecksum the client was built against.
var ErrSpecAbsent = errors.New("coolify: pinned OpenAPI spec not found on disk")

// VerifyBootSpec checks the pinned OpenAPI spec in openapiDir before any command trusts
// a Coolify endpoint. It verifies that sha256(spec) equals both the compiled-in checksum
// and the on-disk .sha256 sidecar, returning an explicit error on any mismatch so the
// command refuses to boot.
//
// It returns ErrSpecAbsent when the directory or spec file is missing; this is a soft
// signal (not a mismatch) the caller can choose to ignore.
func VerifyBootSpec(openapiDir string) error {
	spec, err := os.ReadFile(filepath.Join(openapiDir, specFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrSpecAbsent
		}
		return fmt.Errorf("coolify: read OpenAPI spec: %w", err)
	}
	if err = VerifyOpenAPIChecksum(spec); err != nil {
		return err
	}
	sidecar, err := os.ReadFile(filepath.Join(openapiDir, sidecarFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrSpecAbsent
		}
		return fmt.Errorf("coolify: read OpenAPI sidecar: %w", err)
	}
	fields := strings.Fields(string(sidecar))
	if len(fields) == 0 || fields[0] != openAPIChecksum {
		return fmt.Errorf("coolify: OpenAPI sidecar checksum mismatch: got %q, want %s",
			string(sidecar), openAPIChecksum)
	}
	return nil
}
