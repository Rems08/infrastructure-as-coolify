package config

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
)

// Issue is a single problem found while validating.
type Issue struct {
	File    string
	Message string
}

// Report is the outcome of a validate run.
type Report struct {
	Apps   []string // logical names of successfully validated applications
	Issues []Issue
}

// OK reports whether validation found no issues.
func (r Report) OK() bool { return len(r.Issues) == 0 }

// Validate validates target (a file or a directory) and returns a Report. When strict
// is set, visible `value:` fields are scanned for secret-like content. A returned
// error indicates a fatal problem (e.g. target missing), not a validation failure —
// those live in Report.Issues.
func Validate(target string, strict bool) (Report, error) {
	files, err := collectFiles(target)
	if err != nil {
		return Report{}, err
	}
	var rep Report
	for _, f := range files {
		validateFile(f, strict, &rep)
	}
	sort.Strings(rep.Apps)
	return rep, nil
}

// collectFiles returns the YAML files to validate. A file target is returned as-is; a
// directory is walked for Application resources (other kinds are skipped).
func collectFiles(target string) ([]string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("validate target: %w", err)
	}
	if !info.IsDir() {
		return []string{target}, nil
	}
	var files []string
	walkErr := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		// Unreadable or non-Application files (e.g. coolify.yaml) are skipped in dir
		// mode; a bad Application file surfaces later as a parse issue.
		if kind, _ := PeekKind(path); kind == resource.KindApplication {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %s: %w", target, walkErr)
	}
	return files, nil
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func validateFile(path string, strict bool, rep *Report) {
	app, err := LoadApplication(path)
	if err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if err := app.Validate(); err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if strict {
		scanStrict(path, app, rep)
	}
	rep.Apps = append(rep.Apps, app.Metadata.Name)
}

func scanStrict(path string, app resource.Application, rep *Report) {
	for _, ev := range app.Spec.EnvVars {
		if ev.Value == "" {
			continue
		}
		if reason, ok := DetectSecretLike(ev.Value); ok {
			rep.Issues = append(rep.Issues, Issue{
				File: path,
				Message: fmt.Sprintf(
					"env_var %q uses a plaintext `value` that %s; did you mean `value_secret`?",
					ev.Name, reason),
			})
		}
	}
}

// WriteReport renders a Report to w in human-readable form and returns true when the
// report is clean.
func WriteReport(w io.Writer, rep Report) bool {
	if rep.OK() {
		fmt.Fprintln(w, summaryLine(rep.Apps))
		return true
	}
	for _, iss := range rep.Issues {
		fmt.Fprintf(w, "%s: %s\n", iss.File, iss.Message)
	}
	fmt.Fprintf(w, "\n%d issue(s) found\n", len(rep.Issues))
	return false
}

func summaryLine(apps []string) string {
	noun := "applications"
	if len(apps) == 1 {
		noun = "application"
	}
	return fmt.Sprintf("Validated %d %s: %s (no issues)", len(apps), noun, strings.Join(apps, ", "))
}
