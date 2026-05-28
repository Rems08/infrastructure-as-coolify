package main

import (
	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/docs"
)

func newDocsCmd() *cobra.Command {
	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Documentation tooling",
	}

	var (
		resourceDir string
		outDir      string
	)
	genCmd := &cobra.Command{
		Use:   "gen",
		Short: "Generate reference docs from the resource struct tags",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := docs.Generate(resourceDir, outDir); err != nil {
				return err
			}
			cmd.Printf("Generated %s/application.md and %s/schema.json\n", outDir, outDir)
			return nil
		},
	}
	genCmd.Flags().StringVar(&resourceDir, "resource-dir", "internal/resource", "Directory of resource Go sources")
	genCmd.Flags().StringVar(&outDir, "out-dir", "docs/reference", "Output directory")
	docsCmd.AddCommand(genCmd)
	return docsCmd
}
