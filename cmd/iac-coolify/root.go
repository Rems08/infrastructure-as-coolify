package main

import "github.com/spf13/cobra"

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "iac-coolify",
		Short:         "Declarative Infrastructure as Code for Coolify v4",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newValidateCmd(), newPlanCmd(), newApplyCmd(), newDocsCmd(), newVersionCmd())
	return root
}
