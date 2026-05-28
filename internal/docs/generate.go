// Package docs generates the reference documentation from the iac struct tags in
// internal/resource, keeping docs and code in lock-step (single source of truth). An
// architecture test (generate_test.go) fails if the committed markdown drifts from the
// structs.
package docs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

// FieldDoc is one documented struct field.
type FieldDoc struct {
	YAMLName string
	Doc      string
	Required bool
	Enum     []string
}

// StructDoc is a struct and its documented fields.
type StructDoc struct {
	Name   string
	Fields []FieldDoc
}

// ExtractStructDocs parses the .go files in resourceDir and returns, in file then
// declaration order, every struct that has at least one field carrying a non-empty
// `iac:"doc=..."` tag.
func ExtractStructDocs(resourceDir string) ([]StructDoc, error) {
	entries, err := os.ReadDir(resourceDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", resourceDir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	fset := token.NewFileSet()
	var out []StructDoc
	for _, name := range names {
		f, parseErr := parser.ParseFile(fset, filepath.Join(resourceDir, name), nil, 0)
		if parseErr != nil {
			return nil, fmt.Errorf("parse %s: %w", name, parseErr)
		}
		out = append(out, structsInFile(f)...)
	}
	return out, nil
}

func structsInFile(f *ast.File) []StructDoc {
	var docs []StructDoc
	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			if fields := docFields(st); len(fields) > 0 {
				docs = append(docs, StructDoc{Name: ts.Name.Name, Fields: fields})
			}
		}
	}
	return docs
}

func docFields(st *ast.StructType) []FieldDoc {
	var fields []FieldDoc
	for _, field := range st.Fields.List {
		if field.Tag == nil || len(field.Names) == 0 {
			continue
		}
		tag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
		iac := tag.Get("iac")
		if iac == "" {
			continue
		}
		fd := parseIACTag(iac)
		if fd.Doc == "" {
			continue
		}
		fd.YAMLName = yamlName(tag.Get("yaml"), field.Names[0].Name)
		fields = append(fields, fd)
	}
	return fields
}

func yamlName(yamlTag, fieldName string) string {
	if yamlTag == "" {
		return fieldName
	}
	return strings.Split(yamlTag, ",")[0]
}

// parseIACTag parses `doc=<text>,<flag>,...` where flags are `required` or `enum=a|b`.
// The doc text is the (comma-free) first segment.
func parseIACTag(tag string) FieldDoc {
	var fd FieldDoc
	for _, part := range strings.Split(tag, ",") {
		switch {
		case strings.HasPrefix(part, "doc="):
			fd.Doc = strings.TrimPrefix(part, "doc=")
		case part == "required":
			fd.Required = true
		case strings.HasPrefix(part, "enum="):
			fd.Enum = strings.Split(strings.TrimPrefix(part, "enum="), "|")
		}
	}
	return fd
}
