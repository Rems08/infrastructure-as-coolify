package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Rems08/infrastructure-as-coolify/internal/config"
)

func newValidateCmd() *cobra.Command {
	var (
		configDir string
		strict    bool
	)
	cmd := &cobra.Command{
		Use:   "validate [PATH]",
		Short: "Validate YAML files against the iac-coolify schema",
		Long: "Validate a file or directory of Coolify resource YAML. With --strict, " +
			"visible `value:` fields are scanned for plaintext secrets.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := configDir
			if len(args) == 1 {
				target = args[0]
			}
			rep, err := config.Validate(target, strict)
			if err != nil {
				return err
			}
			if ok := config.WriteReport(cmd.OutOrStdout(), rep); !ok {
				return fmt.Errorf("validation failed: %d issue(s)", len(rep.Issues))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&configDir, "config-dir", "coolify", "Root config directory (used when PATH is omitted)")
	cmd.Flags().BoolVar(&strict, "strict", false, "Detect plaintext secrets in visible values")
	return cmd
}
