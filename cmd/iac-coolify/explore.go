package main

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/apply"
	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/tui"
)

type exploreOptions struct {
	coolifyURL string
	auditLog   string
	configPath string
}

func newExploreCmd() *cobra.Command {
	opts := exploreOptions{}
	cmd := &cobra.Command{
		Use:     "explore [config-path]",
		Aliases: []string{"tui"},
		Short:   "Browse live Coolify state in an interactive terminal UI",
		Long: "Open a terminal browser over the live Coolify instance: walk the project,\n" +
			"environment and resource tree and inspect each resource. Pass an optional config\n" +
			"path to enable the drift view (D) comparing desired YAML against live state, and\n" +
			"trigger application lifecycle actions (R restart, S stop, U start) behind a confirm\n" +
			"prompt. Requires an interactive terminal and Coolify credentials.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				opts.configPath = args[0]
			}
			return runExplore(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.coolifyURL, "coolify-url", "", "Coolify base URL (or COOLIFY_API_URL); token via COOLIFY_API_TOKEN")
	cmd.Flags().StringVar(&opts.auditLog, "audit-log", ".iac-coolify/audit.log", "Append-only audit log path for lifecycle actions")
	return cmd
}

func runExplore(parent context.Context, opts exploreOptions) error {
	if isNonInteractive() {
		return fmt.Errorf("explore requires an interactive terminal (not a pipe or CI)")
	}
	client, err := exploreClient(opts.coolifyURL)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	handler := tui.NewLogHandler(slog.LevelInfo)
	prev := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(prev)

	model := tui.NewModel(ctx, client, client,
		tui.WithConfigPath(opts.configPath),
		tui.WithAuditor(apply.NewAuditor(opts.auditLog)),
	)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	handler.Wire(p.Send)

	_, err = p.Run()
	return err
}

// exploreClient builds a Coolify client and, unlike plan, refuses to run without
// credentials: explore only browses the live instance, so there is no offline mode.
func exploreClient(flagURL string) (*coolify.Client, error) {
	client, online, err := buildClient(flagURL)
	if err != nil {
		return nil, err
	}
	if !online {
		return nil, fmt.Errorf("explore requires Coolify credentials (set COOLIFY_API_URL and COOLIFY_API_TOKEN)")
	}
	return client, nil
}
