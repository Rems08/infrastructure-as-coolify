package secrets_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// allowedRevealPackages lists the only packages permitted to call Secret.Reveal().
// internal/secrets owns the type (and its tests); internal/coolify is the HTTP
// boundary that builds the Authorization header.
var allowedRevealPackages = map[string]bool{
	"internal/secrets": true,
	"internal/coolify": true,
}

type revealSite struct {
	file string
	line int
	pkg  string
}

// TestRevealCallSitesAllowlisted scans the AST of cmd/ + internal/ and fails if
// Secret.Reveal() is called from any package outside the allowlist.
func TestRevealCallSitesAllowlisted(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sites := findRevealCallSites(t, root)
	if len(sites) == 0 {
		t.Fatal("found 0 Reveal() call sites; the scanner is likely broken")
	}
	for _, s := range sites {
		if !allowedRevealPackages[s.pkg] {
			t.Errorf("Secret.Reveal() called outside allowlist: %s:%d (package %s)",
				s.file, s.line, s.pkg)
		}
	}
}

func findRevealCallSites(t *testing.T, root string) []revealSite {
	t.Helper()
	var sites []revealSite
	fset := token.NewFileSet()

	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "dist", "testdata", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Only scan cmd/ and internal/ trees.
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "cmd/") && !strings.HasPrefix(rel, "internal/") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Reveal" || len(call.Args) != 0 {
				return true
			}
			sites = append(sites, revealSite{
				file: rel,
				line: fset.Position(call.Pos()).Line,
				pkg:  pkg,
			})
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	return sites
}
