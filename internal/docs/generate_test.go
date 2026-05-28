package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/docs"
)

const resourceDir = "../resource"

// TestStructFieldsMatchMarkdownHeadings covers critère §7 #9. For every resource it
// re-extracts the documented fields from the structs and asserts that 100% of them have a
// heading in the committed docs/reference/<slug>.md. Regenerate with
// `iac-coolify docs gen` (or `go run ./cmd/iac-coolify docs gen`) when it fails.
func TestStructFieldsMatchMarkdownHeadings(t *testing.T) {
	resources, err := docs.ExtractResourceDocs(resourceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) == 0 {
		t.Fatal("no documented resources found; extractor likely broken")
	}

	for _, rd := range resources {
		mdPath := filepath.Join("..", "..", "docs", "reference", rd.Slug+".md")
		mdBytes, rErr := os.ReadFile(mdPath)
		if rErr != nil {
			t.Fatalf("read %s (run `iac-coolify docs gen`): %v", mdPath, rErr)
		}
		headings := collectHeadings(string(mdBytes))
		for _, s := range rd.Structs {
			for _, f := range s.Fields {
				if !headings[f.YAMLName] {
					t.Errorf("%s.%s has an iac:doc tag but no `### %s` heading in %s.md",
						s.Name, f.YAMLName, f.YAMLName, rd.Slug)
				}
			}
		}
	}
}

// TestGeneratedDocIsUpToDate fails if any committed markdown differs from a fresh render,
// ensuring docs never drift from the structs.
func TestGeneratedDocIsUpToDate(t *testing.T) {
	resources, err := docs.ExtractResourceDocs(resourceDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, rd := range resources {
		want := docs.RenderResource(rd)
		mdPath := filepath.Join("..", "..", "docs", "reference", rd.Slug+".md")
		got, rErr := os.ReadFile(mdPath)
		if rErr != nil {
			t.Fatal(rErr)
		}
		if string(got) != want {
			t.Errorf("docs/reference/%s.md is stale; run `iac-coolify docs gen`", rd.Slug)
		}
	}
}

func collectHeadings(md string) map[string]bool {
	out := make(map[string]bool)
	for _, line := range strings.Split(md, "\n") {
		if name, ok := strings.CutPrefix(line, "### "); ok {
			out[strings.TrimSpace(name)] = true
		}
	}
	return out
}
