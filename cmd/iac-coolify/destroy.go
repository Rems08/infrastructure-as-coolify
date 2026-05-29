package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/config"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/resource"
	"github.com/Rems08/infrastructure-as-coolify/internal/state"
)

type destroyOptions struct {
	target      string
	configDir   string
	only        string
	coolifyURL  string
	output      string
	autoApprove bool
	dryRun      bool
	stateCache  string
	openapiDir  string
	auditLog    string
}

func newDestroyCmd() *cobra.Command {
	opts := destroyOptions{}
	cmd := &cobra.Command{
		Use:   "destroy [PATH]",
		Short: "Delete the Coolify resources described by the config",
		Long: "Remove the declared resources from a live Coolify instance in reverse dependency\n" +
			"order (applications and services first, then environments, then projects). Only\n" +
			"resources that still exist remotely are deleted, so a repeated destroy is a no-op.\n" +
			"Refuses to run in a non-interactive session unless --auto-approve is given; use\n" +
			"--dry-run for an offline preview that deletes nothing.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.target = opts.configDir
			if len(args) == 1 {
				opts.target = args[0]
			}
			return runDestroy(cmd.Context(), cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.configDir, "config-dir", "coolify", "Root config directory (used when PATH is omitted)")
	cmd.Flags().StringVar(&opts.only, "target", "", "Destroy only the resource with this logical name")
	cmd.Flags().StringVar(&opts.coolifyURL, "coolify-url", "", "Coolify base URL (or COOLIFY_API_URL); token via COOLIFY_API_TOKEN")
	cmd.Flags().StringVar(&opts.output, "output", "", "Output format: text|json (default: auto-detect TTY/CI)")
	cmd.Flags().BoolVar(&opts.autoApprove, "auto-approve", false, "Destroy without an interactive prompt (required in CI)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Show the resources that would be deleted without deleting them")
	cmd.Flags().StringVar(&opts.stateCache, "state-cache", "", "Write the resolved UUID map to this JSON file")
	cmd.Flags().StringVar(&opts.openapiDir, "openapi-dir", "testdata/openapi", "Directory of the pinned OpenAPI spec for boot checksum verification")
	cmd.Flags().StringVar(&opts.auditLog, "audit-log", ".iac-coolify/audit.log", "Append-only audit log path")
	return cmd
}

func runDestroy(ctx context.Context, cmd *cobra.Command, opts destroyOptions) error {
	// A non-interactive run must opt in explicitly before any Coolify connection, so CI can
	// never delete silently. A dry-run mutates nothing and is exempt.
	if !opts.dryRun && !opts.autoApprove && isNonInteractive() {
		return fmt.Errorf("destroy: refusing to destroy in a non-interactive session; pass --auto-approve")
	}
	format := resolveFormat(opts.output)
	log := newLogger(format)

	resolved, client, online, err := destroyResolve(ctx, opts, log)
	if err != nil {
		return err
	}

	in, err := loadDeleteInput(opts.target, resolved, opts.only, !online && opts.dryRun)
	if err != nil {
		return err
	}
	ops, err := apply.OrderDelete(in.DeleteOperations())
	if err != nil {
		return err
	}

	if opts.dryRun {
		return writeDestroyOutput(cmd, format, ops, 0, true)
	}
	if !opts.autoApprove {
		if !confirm(cmd, len(ops)) {
			fmt.Fprintln(cmd.OutOrStdout(), "Destroy cancelled.")
			return nil
		}
	}

	eng := apply.NewEngine(client, resolved, apply.NewAuditor(opts.auditLog))
	sum, destroyErr := eng.Apply(ctx, ops)
	_ = writeDestroyOutput(cmd, format, ops, sum.Applied, false)
	if destroyErr != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "iac-coolify:", destroyErr)
		if sum.Applied > 0 {
			return exitErr{code: 2} // partial: some resources deleted before the failure
		}
		return exitErr{code: 1}
	}
	return nil
}

// destroyResolve loads the live UUID map. Destroying (not a dry-run) requires an online
// client; an offline dry-run treats every declared resource as present so the preview lists
// the full teardown.
func destroyResolve(ctx context.Context, opts destroyOptions, log *slog.Logger) (state.Map, *coolify.Client, bool, error) {
	client, online, err := buildClient(opts.coolifyURL)
	if err != nil {
		return nil, nil, false, err
	}
	if !online && !opts.dryRun {
		return nil, nil, false, fmt.Errorf("destroy: requires a Coolify URL and COOLIFY_API_TOKEN (use --dry-run for an offline preview)")
	}
	resolved := state.Map{}
	if online {
		if resolved, err = resolveRemote(ctx, client, planOptions{stateCache: opts.stateCache, openapiDir: opts.openapiDir}, log); err != nil {
			return nil, nil, false, err
		}
	} else {
		log.WarnContext(ctx, "no Coolify URL/token configured; dry-run lists every declared resource as to-destroy")
	}
	return resolved, client, online, nil
}

// loadDeleteInput loads every declared resource under target and pairs it with the live
// state. assumePresent is set for an offline dry-run, where the live state is unknown and
// every declared resource is listed as to-destroy.
func loadDeleteInput(target string, resolved state.Map, only string, assumePresent bool) (apply.DeleteInput, error) {
	projects, err := config.LoadProjects(target)
	if err != nil {
		return apply.DeleteInput{}, err
	}
	envs, err := config.LoadEnvironments(target)
	if err != nil {
		return apply.DeleteInput{}, err
	}
	apps, err := config.LoadApplications(target)
	if err != nil {
		return apply.DeleteInput{}, err
	}
	loaded, err := config.LoadServices(target)
	if err != nil {
		return apply.DeleteInput{}, err
	}
	services := make([]resource.Service, 0, len(loaded))
	for _, ls := range loaded {
		services = append(services, ls.Service)
	}
	return apply.DeleteInput{
		Projects:      projects,
		Environments:  envs,
		Applications:  apps,
		Services:      services,
		Resolved:      resolved,
		Only:          only,
		AssumePresent: assumePresent,
	}, nil
}

func writeDestroyOutput(cmd *cobra.Command, format string, ops []apply.Operation, destroyed int, dryRun bool) error {
	out := applyOutput{DryRun: dryRun, Applied: destroyed, Operations: make([]string, 0, len(ops))}
	for _, op := range ops {
		out.ToDestroy++
		out.Operations = append(out.Operations, string(op.Op)+" "+opLabel(op))
	}
	if format == "json" {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	w := cmd.OutOrStdout()
	for _, line := range out.Operations {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintf(w, "\nDestroy: %d to destroy.\n", out.ToDestroy)
	if !dryRun {
		fmt.Fprintf(w, "Destroy complete. %d destroyed.\n", out.Applied)
	}
	return nil
}
