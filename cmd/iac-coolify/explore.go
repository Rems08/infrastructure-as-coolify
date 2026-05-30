package main

import (
	"context"
	"fmt"
	"log/slog"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/coolify"
	"github.com/Rems08/infrastructure-as-coolify/internal/tui"
)

type exploreOptions struct {
	coolifyURL string
}

func newExploreCmd() *cobra.Command {
	opts := exploreOptions{}
	cmd := &cobra.Command{
		Use:     "explore",
		Aliases: []string{"tui"},
		Short:   "Browse live Coolify state in an interactive terminal UI",
		Long: "Open a read-only terminal browser over the live Coolify instance: walk the\n" +
			"project, environment and resource tree and inspect each resource. Requires an\n" +
			"interactive terminal and Coolify credentials; it never mutates anything.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExplore(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.coolifyURL, "coolify-url", "", "Coolify base URL (or COOLIFY_API_URL); token via COOLIFY_API_TOKEN")
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

	p := tea.NewProgram(tui.NewModel(ctx, client), tea.WithAltScreen(), tea.WithContext(ctx))
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
