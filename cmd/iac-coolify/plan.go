package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/plan"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/secrets"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

// exitErr requests a specific process exit code (see main.exitCoder).
type exitErr struct{ code int }

func (e exitErr) Error() string { return fmt.Sprintf("exit code %d", e.code) }
func (e exitErr) ExitCode() int { return e.code }

type planOptions struct {
	target       string
	coolifyURL   string
	output       string
	detailedExit bool
	stateCache   string
	openapiDir   string
}

func newPlanCmd() *cobra.Command {
	opts := planOptions{}
	cmd := &cobra.Command{
		Use:   "plan [PATH]",
		Short: "Show the changes required to reach the desired state",
		Long: "Parse the resource YAML, resolve live Coolify state and print a Terraform-style\n" +
			"diff. With no Coolify URL/token configured, plan runs offline and treats every\n" +
			"resource as a creation.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.target = "coolify"
			if len(args) == 1 {
				opts.target = args[0]
			}
			return runPlan(cmd.Context(), cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.coolifyURL, "coolify-url", "", "Coolify base URL (or COOLIFY_API_URL); token via COOLIFY_API_TOKEN")
	cmd.Flags().StringVar(&opts.output, "output", "", "Output format: text|json (default: auto-detect TTY/CI)")
	cmd.Flags().BoolVar(&opts.detailedExit, "detailed-exitcode", false, "Exit 0 (no changes), 2 (changes), 1 (error)")
	cmd.Flags().StringVar(&opts.stateCache, "state-cache", "", "Write the resolved UUID map to this JSON file")
	cmd.Flags().StringVar(&opts.openapiDir, "openapi-dir", "testdata/openapi", "Directory of the pinned OpenAPI spec for boot checksum verification")
	return cmd
}

func runPlan(ctx context.Context, cmd *cobra.Command, opts planOptions) error {
	format := resolveFormat(opts.output)
	log := newLogger(format)

	apps, err := config.LoadApplications(opts.target)
	if err != nil {
		return err
	}

	client, online, err := buildClient(opts.coolifyURL)
	if err != nil {
		return err
	}
	rmap := state.Map{}
	if online {
		if rmap, err = resolveRemote(ctx, client, opts, log); err != nil {
			return err
		}
	} else {
		log.WarnContext(ctx, "no Coolify URL/token configured; planning offline (all resources treated as new)")
	}

	dbs, err := config.LoadDatabases(opts.target)
	if err != nil {
		return err
	}

	var p plan.Plan
	for _, app := range apps {
		actual, aErr := remoteApplication(ctx, client, rmap, app)
		if aErr != nil {
			return aErr
		}
		p.Add(plan.FromApplication(app), actual)
	}
	for _, db := range dbs {
		actual, aErr := remoteDatabase(ctx, client, rmap, db)
		if aErr != nil {
			return aErr
		}
		p.Add(plan.FromDatabase(db), actual)
	}

	if err := writePlan(cmd, format, p); err != nil {
		return err
	}
	if opts.detailedExit && p.HasChanges() {
		return exitErr{code: 2}
	}
	return nil
}

// resolveRemote verifies the pinned spec, resolves the live UUID map and optionally
// caches it.
func resolveRemote(ctx context.Context, client *coolify.Client, opts planOptions, log *slog.Logger) (state.Map, error) {
	if vErr := coolify.VerifyBootSpec(opts.openapiDir); vErr != nil {
		if !errors.Is(vErr, coolify.ErrSpecAbsent) {
			return nil, vErr
		}
		log.WarnContext(ctx, "pinned OpenAPI spec not found; skipping checksum verification", "dir", opts.openapiDir)
	}
	rmap, err := state.Resolve(ctx, client)
	if err != nil {
		return nil, err
	}
	if opts.stateCache != "" {
		if sErr := rmap.Save(opts.stateCache, coolify.OpenAPIChecksum(), time.Now()); sErr != nil {
			return nil, sErr
		}
		log.InfoContext(ctx, "state cache written", "path", opts.stateCache, "resources", len(rmap))
	}
	return rmap, nil
}

// remoteApplication returns the live projection of app, or nil when it does not exist
// remotely (or when planning offline with a nil client).
func remoteApplication(ctx context.Context, client *coolify.Client, rmap state.Map, app resource.Application) (*plan.Resource, error) {
	if client == nil {
		return nil, nil
	}
	key := state.ResourceKey{
		Project:     app.Metadata.Project,
		Environment: app.Metadata.Environment,
		Kind:        resource.KindApplication,
		Name:        app.Metadata.Name,
	}
	uuid, ok := rmap.Lookup(key)
	if !ok {
		return nil, nil
	}
	live, err := client.GetApplication(ctx, uuid)
	if err != nil {
		return nil, err
	}
	r := plan.FromRemoteApplication(live)
	return &r, nil
}

// remoteDatabase returns the live projection of db, or nil when it does not exist remotely
// (or when planning offline with a nil client). Databases are resolved by name alone (the
// resolver keys them that way), so the lookup carries no project or environment.
func remoteDatabase(ctx context.Context, client *coolify.Client, rmap state.Map, db resource.Database) (*plan.Resource, error) {
	if client == nil {
		return nil, nil
	}
	uuid, ok := rmap.Lookup(state.ResourceKey{Kind: resource.KindDatabase, Name: db.Metadata.Name})
	if !ok {
		return nil, nil
	}
	live, err := client.GetDatabase(ctx, uuid)
	if err != nil {
		return nil, err
	}
	r := plan.FromRemoteDatabase(live)
	return &r, nil
}

func writePlan(cmd *cobra.Command, format string, p plan.Plan) error {
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(p.Output())
	}
	fmt.Fprint(cmd.OutOrStdout(), p.RenderText())
	return nil
}

// buildClient returns a Coolify client and whether it is online. With neither URL nor
// token set, it returns online=false (offline plan). A half-configured pair is an error.
func buildClient(flagURL string) (*coolify.Client, bool, error) {
	url := flagURL
	if url == "" {
		url = os.Getenv("COOLIFY_API_URL")
	}
	_, hasToken := os.LookupEnv("COOLIFY_API_TOKEN")
	switch {
	case url == "" && !hasToken:
		return nil, false, nil
	case url == "" || !hasToken:
		return nil, false, fmt.Errorf("plan: set both a Coolify URL (--coolify-url or COOLIFY_API_URL) and COOLIFY_API_TOKEN, or neither for an offline plan")
	}
	tok, err := secrets.NewFromEnv("COOLIFY_API_TOKEN")
	if err != nil {
		return nil, false, err
	}
	opts := coolify.Options{BaseURL: url, Token: tok}
	if id := os.Getenv("CF_ACCESS_CLIENT_ID"); id != "" {
		sec, sErr := secrets.NewFromEnv("CF_ACCESS_CLIENT_SECRET")
		if sErr != nil {
			return nil, false, sErr
		}
		opts.CFAccessClientID = id
		opts.CFAccessClientSecret = sec
	}
	c, err := coolify.NewClient(opts)
	return c, true, err
}

func resolveFormat(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if os.Getenv("CI") != "" {
		return "json"
	}
	if fi, err := os.Stdout.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		return "json" // piped/redirected
	}
	return "text"
}

func newLogger(format string) *slog.Logger {
	if format == "json" {
		return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
}
