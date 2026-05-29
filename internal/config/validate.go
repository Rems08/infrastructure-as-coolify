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

// Report is the outcome of a validate run. Each slice holds the logical names of the
// successfully validated resources of that kind.
type Report struct {
	Projects     []string
	Environments []string
	Apps         []string
	Services     []string
	Databases    []string
	EnvVars      []string
	Issues       []Issue
}

// OK reports whether validation found no issues.
func (r Report) OK() bool { return len(r.Issues) == 0 }

// kindFile pairs a YAML file with its declared kind.
type kindFile struct {
	path string
	kind string
}

// Validate validates target (a file or a directory) and returns a Report. When strict
// is set, visible `value:` fields are scanned for secret-like content. A returned error
// indicates a fatal problem (e.g. target missing), not a validation failure — those live
// in Report.Issues.
func Validate(target string, strict bool) (Report, error) {
	files, err := collectFiles(target)
	if err != nil {
		return Report{}, err
	}
	var rep Report
	root := composeRoot(target)
	for _, f := range files {
		validateFile(f, strict, root, &rep)
	}
	sort.Strings(rep.Projects)
	sort.Strings(rep.Environments)
	sort.Strings(rep.Apps)
	sort.Strings(rep.Services)
	sort.Strings(rep.Databases)
	sort.Strings(rep.EnvVars)
	return rep, nil
}

// collectFiles returns the YAML files to validate, each tagged with its kind. A file
// target is returned as-is (its kind peeked); a directory is walked for known resource
// kinds (other kinds, e.g. the project coolify.yaml, are skipped).
func collectFiles(target string) ([]kindFile, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, fmt.Errorf("validate target: %w", err)
	}
	if !info.IsDir() {
		kind, _ := PeekKind(target) // empty kind surfaces as an issue in validateFile
		return []kindFile{{path: target, kind: kind}}, nil
	}
	var files []kindFile
	walkErr := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		if kind, _ := PeekKind(path); isKnownKind(kind) {
			files = append(files, kindFile{path: path, kind: kind})
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %s: %w", target, walkErr)
	}
	return files, nil
}

func isKnownKind(kind string) bool {
	switch kind {
	case resource.KindProject, resource.KindEnvironment,
		resource.KindApplication, resource.KindService,
		resource.KindDatabase, resource.KindEnvVar:
		return true
	default:
		return false
	}
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

func validateFile(kf kindFile, strict bool, root string, rep *Report) {
	switch kf.kind {
	case resource.KindProject:
		validateProject(kf.path, rep)
	case resource.KindEnvironment:
		validateEnvironment(kf.path, rep)
	case resource.KindApplication:
		validateApplication(kf.path, strict, rep)
	case resource.KindService:
		validateService(kf.path, root, rep)
	case resource.KindDatabase:
		validateDatabase(kf.path, rep)
	case resource.KindEnvVar:
		validateEnvVar(kf.path, strict, rep)
	default:
		rep.Issues = append(rep.Issues, Issue{
			File:    kf.path,
			Message: fmt.Sprintf("unsupported or missing kind %q", kf.kind),
		})
	}
}

func validateProject(path string, rep *Report) {
	p, err := LoadProject(path)
	if err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if err := p.Validate(); err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	rep.Projects = append(rep.Projects, p.Metadata.Name)
}

func validateEnvironment(path string, rep *Report) {
	e, err := LoadEnvironment(path)
	if err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if err := e.Validate(); err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	rep.Environments = append(rep.Environments, e.Metadata.Name)
}

func validateApplication(path string, strict bool, rep *Report) {
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
		scanStrict(path, app.Spec.EnvVars, rep)
	}
	rep.Apps = append(rep.Apps, app.Metadata.Name)
}

func validateService(path, root string, rep *Report) {
	svc, err := LoadService(path)
	if err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if err := svc.Validate(); err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if svc.Spec.HasComposePath() {
		if err := resource.ValidateComposePath(root, filepath.Dir(path), svc.Spec.DockerComposePath); err != nil {
			rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
			return
		}
	}
	rep.Services = append(rep.Services, svc.Metadata.Name)
}

func validateDatabase(path string, rep *Report) {
	db, err := LoadDatabase(path)
	if err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if err := db.Validate(); err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	rep.Databases = append(rep.Databases, db.Metadata.Name)
}

func validateEnvVar(path string, strict bool, rep *Report) {
	ev, err := LoadEnvVar(path)
	if err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if err := ev.Validate(); err != nil {
		rep.Issues = append(rep.Issues, Issue{File: path, Message: err.Error()})
		return
	}
	if strict {
		scanStrict(path, ev.Spec.Vars, rep)
	}
	rep.EnvVars = append(rep.EnvVars, ev.Metadata.Name)
}

func scanStrict(path string, entries []resource.EnvVarEntry, rep *Report) {
	for _, ev := range entries {
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
		fmt.Fprintln(w, summaryLine(rep))
		return true
	}
	for _, iss := range rep.Issues {
		fmt.Fprintf(w, "%s: %s\n", iss.File, iss.Message)
	}
	fmt.Fprintf(w, "\n%d issue(s) found\n", len(rep.Issues))
	return false
}

// summaryLine renders the success line. An application-only run keeps the W1 format with
// names ("Validated 1 application: web"); a mixed run lists per-kind counts joined by
// " + " ("Validated 1 application + 1 envvar").
func summaryLine(rep Report) string {
	type group struct {
		noun  string
		names []string
	}
	groups := []group{
		{"project", rep.Projects},
		{"environment", rep.Environments},
		{"application", rep.Apps},
		{"service", rep.Services},
		{"database", rep.Databases},
		{"envvar", rep.EnvVars},
	}
	var present []group
	for _, g := range groups {
		if len(g.names) > 0 {
			present = append(present, g)
		}
	}
	if len(present) == 0 {
		return "Validated 0 resources (no issues)"
	}
	if len(present) == 1 && present[0].noun == "application" {
		g := present[0]
		return fmt.Sprintf("Validated %d %s: %s (no issues)",
			len(g.names), plural(g.noun, len(g.names)), strings.Join(g.names, ", "))
	}
	segs := make([]string, 0, len(present))
	for _, g := range present {
		segs = append(segs, fmt.Sprintf("%d %s", len(g.names), plural(g.noun, len(g.names))))
	}
	return fmt.Sprintf("Validated %s (no issues)", strings.Join(segs, " + "))
}

func plural(noun string, n int) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}
