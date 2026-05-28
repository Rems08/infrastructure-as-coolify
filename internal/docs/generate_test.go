package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Rems08/infrastructure-as-coolify/internal/docs"
)

const resourceDir = "../resource"

// TestStructFieldsMatchMarkdownHeadings covers critère §7 #9. It re-extracts the
// documented fields from the resource structs and asserts that 100% of them have a
// heading in the committed docs/reference/application.md. Regenerate with
// `iac-coolify docs gen` (or `go run ./cmd/iac-coolify docs gen`) when it fails.
func TestStructFieldsMatchMarkdownHeadings(t *testing.T) {
	structs, err := docs.ExtractStructDocs(resourceDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(structs) == 0 {
		t.Fatal("no documented structs found; extractor likely broken")
	}

	mdPath := filepath.Join("..", "..", "docs", "reference", "application.md")
	mdBytes, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatalf("read %s (run `iac-coolify docs gen`): %v", mdPath, err)
	}
	headings := collectHeadings(string(mdBytes))

	for _, s := range structs {
		for _, f := range s.Fields {
			if !headings[f.YAMLName] {
				t.Errorf("%s.%s has an iac:doc tag but no `### %s` heading in application.md",
					s.Name, f.YAMLName, f.YAMLName)
			}
		}
	}
}

// TestGeneratedDocIsUpToDate fails if the committed markdown differs from a fresh
// render, ensuring docs never drift from the structs.
func TestGeneratedDocIsUpToDate(t *testing.T) {
	structs, err := docs.ExtractStructDocs(resourceDir)
	if err != nil {
		t.Fatal(err)
	}
	want := docs.RenderMarkdown(structs)
	mdPath := filepath.Join("..", "..", "docs", "reference", "application.md")
	got, err := os.ReadFile(mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("docs/reference/application.md is stale; run `iac-coolify docs gen`")
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
