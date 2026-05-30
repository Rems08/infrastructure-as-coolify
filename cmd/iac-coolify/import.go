package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/importer"
)

type importOptions struct {
	coolifyURL     string
	defaultNetwork string
	envFilter      []string
	force          bool
	dir            string
}

func newImportCmd() *cobra.Command {
	opts := importOptions{}
	cmd := &cobra.Command{
		Use:   "import [config-path]",
		Short: "Reverse-engineer a live Coolify instance into local YAML manifests",
		Long: "Detect the Coolify instance from the configured credentials, enumerate its\n" +
			"applications and databases, and scaffold matching YAML manifests under the given\n" +
			"directory (default: the current directory). Runs non-interactively (CI-friendly).\n\n" +
			"Secrets are never written: each environment variable becomes a ${env:KEY} reference\n" +
			"and each database password a ${env:NAME_PASSWORD} reference, so the live values stay\n" +
			"out of the manifests — populate them in your own .env. Services are not imported\n" +
			"(the API does not expose enough to rebuild them) and git-based applications are\n" +
			"written partially (the repository URL is not exposed); the report lists both.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.dir = "."
			if len(args) == 1 {
				opts.dir = args[0]
			}
			return runImport(cmd.Context(), cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.coolifyURL, "coolify-url", "", "Coolify base URL (or COOLIFY_API_URL); token via COOLIFY_API_TOKEN")
	cmd.Flags().StringVar(&opts.defaultNetwork, "default-network", "coolify", "Docker network written on every imported resource (not exposed by the API)")
	cmd.Flags().StringSliceVar(&opts.envFilter, "env", nil, "Import only one or more environment names (repeat for multiple)")
	cmd.Flags().BoolVar(&opts.force, "force", false, "Overwrite existing manifests instead of refusing")
	return cmd
}

func runImport(ctx context.Context, cmd *cobra.Command, opts importOptions) error {
	client, err := exploreClient(opts.coolifyURL)
	if err != nil {
		return err
	}
	rep, err := importer.Run(ctx, client, importer.Options{
		Dir:            opts.dir,
		DefaultNetwork: opts.defaultNetwork,
		EnvFilter:      opts.envFilter,
		Force:          opts.force,
		APIURL:         coolifyURL(opts.coolifyURL),
	})
	fmt.Fprint(cmd.OutOrStdout(), rep.RenderText())
	return err
}

// coolifyURL resolves the instance URL recorded in the scaffolded root manifest, preferring
// the flag and falling back to the environment, exactly as buildClient resolves it.
func coolifyURL(flagURL string) string {
	if flagURL != "" {
		return flagURL
	}
	return os.Getenv("COOLIFY_API_URL")
}
