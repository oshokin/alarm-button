package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

// AttachCobraVersionCommand attaches a `version` subcommand to the provided root command.
// It supports human-readable and short machine-readable output.
func AttachCobraVersionCommand(root *cobra.Command) {
	var short bool

	// Subcommand: `version`.
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information.",
		Long:  "Print detailed version information including semantic version and commit hash.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if short {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), Short())
				return err
			}

			_, err := fmt.Fprintln(cmd.OutOrStdout(), Full())

			return err
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "print semantic version only")
	root.AddCommand(cmd)
}
