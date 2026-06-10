package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

type applyOptions struct {
	target      string
	configDir   string
	only        string
	envFilter   []string
	coolifyURL  string
	output      string
	autoApprove bool
	dryRun      bool
	parallelism int
	stateCache  string
	openapiDir  string
	auditLog    string
	envFile     string
}

func newApplyCmd() *cobra.Command {
	opts := applyOptions{}
	cmd := &cobra.Command{
		Use:   "apply [PATH]",
		Short: "Create or update Coolify resources to match the desired state",
		Long: "Reconcile the resource YAML with a live Coolify instance: projects and\n" +
			"environments are created before the applications that depend on them. Refuses to\n" +
			"run in a non-interactive session unless --auto-approve is given. Use --dry-run for\n" +
			"an offline preview that mutates nothing.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.target = opts.configDir
			if len(args) == 1 {
				opts.target = args[0]
			}
			return runApply(cmd.Context(), cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.configDir, "config-dir", "coolify", "Root config directory (used when PATH is omitted)")
	cmd.Flags().StringVar(&opts.only, "target", "", "Apply only the resource with this logical name")
	cmd.Flags().StringSliceVar(&opts.envFilter, "env", nil, "Filter to one or more environment names (repeat for multiple)")
	cmd.Flags().StringVar(&opts.coolifyURL, "coolify-url", "", "Coolify base URL (or COOLIFY_API_URL); token via COOLIFY_API_TOKEN")
	cmd.Flags().StringVar(&opts.output, "output", "", "Output format: text|json (default: auto-detect TTY/CI)")
	cmd.Flags().BoolVar(&opts.autoApprove, "auto-approve", false, "Apply without an interactive prompt (required in CI)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show the planned operations without applying them")
	cmd.Flags().IntVar(&opts.parallelism, "parallelism", 1, "Concurrent operations (only 1 is supported)")
	cmd.Flags().StringVar(&opts.stateCache, "state-cache", "", "Write the resolved UUID map to this JSON file")
	cmd.Flags().StringVar(&opts.openapiDir, "openapi-dir", "testdata/openapi", "Directory of the pinned OpenAPI spec for boot checksum verification")
	cmd.Flags().StringVar(&opts.auditLog, "audit-log", ".iac-coolify/audit.log", "Append-only audit log path")
	cmd.Flags().StringVar(&opts.envFile, "env-file", "", "Load ${env:} values from a dotenv file before resolving (real env vars win)")
	return cmd
}

func runApply(ctx context.Context, cmd *cobra.Command, opts applyOptions) error {
	if opts.parallelism != 1 {
		return fmt.Errorf("apply: only --parallelism=1 is supported in this release")
	}
	if _, err := loadEnvFile(opts.envFile); err != nil {
		return fmt.Errorf("apply: %w", err)
	}
	// A non-interactive run must opt in explicitly, before any Coolify connection, so CI
	// can never apply silently. A dry-run mutates nothing and is exempt.
	if !opts.dryRun && !opts.autoApprove && isNonInteractive() {
		return fmt.Errorf("apply: refusing to apply in a non-interactive session; pass --auto-approve")
	}
	format := resolveFormat(opts.output)
	log := newLogger(format)

	resolved, client, err := applyResolve(ctx, opts, log)
	if err != nil {
		return err
	}
	ops, err := buildOperations(ctx, cmd, client, resolved, opts)
	if err != nil {
		return err
	}
	ordered, err := apply.OrderApply(ops)
	if err != nil {
		return err
	}

	if opts.dryRun {
		return writeApplyOutput(cmd, format, ordered, 0, true)
	}
	if !opts.autoApprove {
		if !confirm(cmd, len(ordered)) {
			fmt.Fprintln(cmd.OutOrStdout(), "Apply cancelled.")
			return nil
		}
	}

	eng := apply.NewEngine(client, resolved, apply.NewAuditor(opts.auditLog))
	sum, applyErr := eng.Apply(ctx, ordered)
	_ = writeApplyOutput(cmd, format, ordered, sum.Applied, false)
	if applyErr != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "iac-coolify:", applyErr)
		if sum.Applied > 0 {
			return exitErr{code: 2} // partial success: some resources changed
		}
		return exitErr{code: 1}
	}
	return nil
}

// applyResolve loads the live UUID map. Applying (not a dry-run) requires an online
// client; a dry-run runs offline and treats every resource as a creation.
func applyResolve(ctx context.Context, opts applyOptions, log *slog.Logger) (state.Map, *coolify.Client, error) {
	client, online, err := buildClient(opts.coolifyURL)
	if err != nil {
		return nil, nil, err
	}
	if !online && !opts.dryRun {
		return nil, nil, fmt.Errorf("apply: requires a Coolify URL and COOLIFY_API_TOKEN (use --dry-run for an offline preview)")
	}
	resolved := state.Map{}
	if online {
		if resolved, err = resolveRemote(ctx, client, planOptions{stateCache: opts.stateCache, openapiDir: opts.openapiDir}, log); err != nil {
			return nil, nil, err
		}
	} else {
		log.WarnContext(ctx, "no Coolify URL/token configured; dry-run treats every resource as new")
	}
	return resolved, client, nil
}

// buildOperations turns the desired resources into the create/update operations needed to
// converge. Already-present projects and environments are skipped; applications are
// diffed against their live state.
func buildOperations(ctx context.Context, cmd *cobra.Command, client *coolify.Client, resolved state.Map, opts applyOptions) ([]apply.Operation, error) {
	projects, err := config.LoadProjects(opts.target)
	if err != nil {
		return nil, err
	}
	envs, err := config.LoadEnvironments(opts.target)
	if err != nil {
		return nil, err
	}
	apps, err := config.LoadApplications(opts.target)
	if err != nil {
		return nil, err
	}
	services, err := config.LoadServices(opts.target)
	if err != nil {
		return nil, err
	}
	dbs, err := config.LoadDatabases(opts.target)
	if err != nil {
		return nil, err
	}

	// Validate the selection against the full, unfiltered set so a misspelled --target/--env
	// is caught before any narrowing.
	if vErr := validateSelection(cmd, opts.only, opts.envFilter, opts.target, projects, envs, apps, services, dbs); vErr != nil {
		return nil, vErr
	}
	envs = filterByEnv(envs, opts.envFilter, func(e resource.Environment) string { return e.Metadata.Name })
	apps = filterByEnv(apps, opts.envFilter, func(a resource.Application) string { return a.Metadata.Environment })
	services = filterByEnv(services, opts.envFilter, func(ls config.LoadedService) string { return ls.Service.Metadata.Environment })
	dbs = filterByEnv(dbs, opts.envFilter, func(d resource.Database) string { return d.Metadata.Environment })
	apps = filterByName(apps, opts.only, func(a resource.Application) string { return a.Metadata.Name })
	services = filterByName(services, opts.only, func(ls config.LoadedService) string { return ls.Service.Metadata.Name })
	dbs = filterByName(dbs, opts.only, func(d resource.Database) string { return d.Metadata.Name })

	// Load keeps ${env:} references unresolved; apply is the one place they are bound to a
	// value, so an unset env var fails here with a clear message instead of pushing an empty
	// value. Resolve only what survived the --env/--target filters, so a scoped apply never
	// needs env values for resources it is not touching. destroy never calls this.
	if err = config.ResolveSecrets(apps, services, dbs); err != nil {
		return nil, err
	}

	ops := projectEnvOps(projects, envs, resolved, opts.only)
	appOps, err := applicationOps(ctx, client, resolved, apps, opts.only)
	if err != nil {
		return nil, err
	}
	ops = append(ops, appOps...)
	ops = append(ops, serviceOps(services, resolved, opts.only)...)
	dbOps, err := databaseOps(ctx, client, resolved, dbs, opts.only)
	if err != nil {
		return nil, err
	}
	return append(ops, dbOps...), nil
}

// validateSelection rejects a --env/--target combination that matches no declared resource
// before any Coolify call is made. Projects are cross-environment, so they count toward a name
// selection but never restrict the environment universe.
func validateSelection(cmd *cobra.Command, only string, envFilter []string, path string, projects []resource.Project, envs []resource.Environment, apps []resource.Application, services []config.LoadedService, dbs []resource.Database) error {
	scope := newEnvScope(only, envFilter)
	for _, p := range projects {
		scope.addCrossEnv(p.Metadata.Name)
	}
	for _, e := range envs {
		scope.add(e.Metadata.Name, e.Metadata.Name)
	}
	for _, a := range apps {
		scope.add(a.Metadata.Name, a.Metadata.Environment)
	}
	for _, ls := range services {
		scope.add(ls.Service.Metadata.Name, ls.Service.Metadata.Environment)
	}
	for _, d := range dbs {
		scope.add(d.Metadata.Name, d.Metadata.Environment)
	}
	return scope.validate(cmd, path)
}

// projectEnvOps returns the create operations for projects and environments not yet
// present remotely.
func projectEnvOps(projects []resource.Project, envs []resource.Environment, resolved state.Map, only string) []apply.Operation {
	var ops []apply.Operation
	for _, p := range projects {
		if selected(only, p.Metadata.Name) && !resolved.Has(state.ResourceKey{Kind: resource.KindProject, Name: p.Metadata.Name}) {
			ops = append(ops, apply.CreateProjectOp(p))
		}
	}
	for _, e := range envs {
		key := state.ResourceKey{Project: e.Metadata.Project, Kind: resource.KindEnvironment, Name: e.Metadata.Name}
		if selected(only, e.Metadata.Name) && !resolved.Has(key) {
			ops = append(ops, apply.CreateEnvironmentOp(e))
		}
	}
	return ops
}

// applicationOps diffs each application against its live state and returns the operations
// needed to converge the ones that differ.
func applicationOps(ctx context.Context, client *coolify.Client, resolved state.Map, apps []resource.Application, only string) ([]apply.Operation, error) {
	var ops []apply.Operation
	for _, app := range apps {
		if !selected(only, app.Metadata.Name) {
			continue
		}
		op, change, err := applicationOp(ctx, client, resolved, app)
		if err != nil {
			return nil, err
		}
		if change {
			ops = append(ops, op)
		}
	}
	return ops, nil
}

// serviceOps returns the create operations for services not yet present remotely. Services
// have no field-level diff yet, so an already-resolved service is left untouched.
func serviceOps(services []config.LoadedService, resolved state.Map, only string) []apply.Operation {
	var ops []apply.Operation
	for _, ls := range services {
		m := ls.Service.Metadata
		key := state.ResourceKey{Project: m.Project, Environment: m.Environment, Kind: resource.KindService, Name: m.Name}
		if selected(only, m.Name) && !resolved.Has(key) {
			ops = append(ops, apply.ServiceOp(apply.OpCreate, ls.Service, ls.ComposeRaw, nil))
		}
	}
	return ops
}

// applicationOp diffs an application against its live state and returns the operation to
// converge it, or change=false when it is already up to date.
func applicationOp(ctx context.Context, client *coolify.Client, resolved state.Map, app resource.Application) (apply.Operation, bool, error) {
	actual, err := remoteApplication(ctx, client, resolved, app)
	if err != nil {
		return apply.Operation{}, false, err
	}
	changes := plan.Diff(plan.FromApplication(app), actual)
	if actual == nil {
		return apply.ApplicationOp(apply.OpCreate, app, changes), true, nil
	}
	if len(changes) == 0 {
		return apply.Operation{}, false, nil
	}
	return apply.ApplicationOp(apply.OpUpdate, app, changes), true, nil
}

// databaseOps diffs each database against its live state and returns the operations needed
// to converge the ones that differ.
func databaseOps(ctx context.Context, client *coolify.Client, resolved state.Map, dbs []resource.Database, only string) ([]apply.Operation, error) {
	var ops []apply.Operation
	for _, db := range dbs {
		if !selected(only, db.Metadata.Name) {
			continue
		}
		op, change, err := databaseOp(ctx, client, resolved, db)
		if err != nil {
			return nil, err
		}
		if change {
			ops = append(ops, op)
		}
	}
	return ops, nil
}

// databaseOp diffs a database against its live state and returns the operation to converge
// it, or change=false when it is already up to date.
func databaseOp(ctx context.Context, client *coolify.Client, resolved state.Map, db resource.Database) (apply.Operation, bool, error) {
	actual, err := remoteDatabase(ctx, client, resolved, db)
	if err != nil {
		return apply.Operation{}, false, err
	}
	changes := plan.Diff(plan.FromDatabase(db), actual)
	if actual == nil {
		return apply.DatabaseOp(apply.OpCreate, db, changes), true, nil
	}
	if len(changes) == 0 {
		return apply.Operation{}, false, nil
	}
	return apply.DatabaseOp(apply.OpUpdate, db, changes), true, nil
}

func selected(only, name string) bool { return only == "" || only == name }

type applyOutput struct {
	DryRun     bool     `json:"dry_run"`
	ToAdd      int      `json:"to_add"`
	ToChange   int      `json:"to_change"`
	ToDestroy  int      `json:"to_destroy"`
	Applied    int      `json:"applied"`
	Operations []string `json:"operations"`
}

func writeApplyOutput(cmd *cobra.Command, format string, ops []apply.Operation, applied int, dryRun bool) error {
	out := applyOutput{DryRun: dryRun, Applied: applied, Operations: make([]string, 0, len(ops))}
	for _, op := range ops {
		switch op.Op {
		case apply.OpCreate:
			out.ToAdd++
		case apply.OpUpdate:
			out.ToChange++
		case apply.OpDelete:
			out.ToDestroy++
		}
		out.Operations = append(out.Operations, string(op.Op)+" "+opLabel(op))
	}
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	return writeApplyText(cmd, out, dryRun)
}

func writeApplyText(cmd *cobra.Command, out applyOutput, dryRun bool) error {
	w := cmd.OutOrStdout()
	for _, line := range out.Operations {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintf(w, "\nApply: %d to add, %d to change, %d to destroy.\n", out.ToAdd, out.ToChange, out.ToDestroy)
	if !dryRun {
		fmt.Fprintf(w, "Apply complete. %d applied.\n", out.Applied)
	}
	return nil
}

func opLabel(op apply.Operation) string {
	segs := []string{op.Kind}
	for _, s := range []string{op.Project, op.Environment, op.Name} {
		if s != "" {
			segs = append(segs, s)
		}
	}
	return strings.Join(segs, "/")
}

// isNonInteractive reports whether apply must not prompt: explicitly in CI, or when stdout
// is not a terminal (piped/redirected).
func isNonInteractive() bool {
	if os.Getenv("CI") != "" {
		return true
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

func confirm(cmd *cobra.Command, n int) bool {
	fmt.Fprintf(cmd.OutOrStdout(), "\nApply %d change(s)? [y/N] ", n)
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
