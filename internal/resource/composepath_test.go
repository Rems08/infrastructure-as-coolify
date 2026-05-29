package resource_test

import (
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

func TestServiceComposePathRejectsAbsolute(t *testing.T) {
	root := "/repo/services"
	base := "/repo/services"
	for _, abs := range []string{"/etc/shadow", "/etc/passwd", "/var/run/secrets/token"} {
		err := resource.ValidateComposePath(root, base, abs)
		if err == nil || !strings.Contains(err.Error(), "must be relative") {
			t.Errorf("ValidateComposePath(%q) = %v, want 'must be relative'", abs, err)
		}
	}
}

func TestServiceComposePathRejectsTraversal(t *testing.T) {
	root := "/repo/services"
	base := "/repo/services"
	for _, rel := range []string{"../secret.yml", "../../etc/passwd", "../../../../../../etc/shadow"} {
		err := resource.ValidateComposePath(root, base, rel)
		if err == nil || !strings.Contains(err.Error(), "must not escape") {
			t.Errorf("ValidateComposePath(%q) = %v, want 'must not escape'", rel, err)
		}
	}
}

func TestServiceComposePathRejectsControlChars(t *testing.T) {
	root := "/repo/services"
	base := "/repo/services"
	cases := []string{"compose-with-null\x00byte.yml", "compose\ttab.yml", "compose\nnewline.yml"}
	for _, rel := range cases {
		err := resource.ValidateComposePath(root, base, rel)
		if err == nil || !strings.Contains(err.Error(), "control characters") {
			t.Errorf("ValidateComposePath(%q) = %v, want 'control characters'", rel, err)
		}
	}
}

func TestServiceComposePathAcceptsValid(t *testing.T) {
	// root is the repository tree; the YAML lives one level down in services/.
	root := "/repo"
	base := "/repo/services"
	cases := []string{
		"./compose.yml",
		"compose.yml",
		"subdir/compose.yml",
		"../sibling/compose.yml", // escapes services/ but stays inside /repo
	}
	for _, rel := range cases {
		if err := resource.ValidateComposePath(root, base, rel); err != nil {
			t.Errorf("ValidateComposePath(%q) = %v, want nil (within root)", rel, err)
		}
	}
}
