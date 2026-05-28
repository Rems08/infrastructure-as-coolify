package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is overridden at build time via -ldflags "-X main.version=v0.1.0".
var version = "dev"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the iac-coolify version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "iac-coolify "+version)
			return err
		},
	}
}
