package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

// AttachCobraVersionCommand attaches a `version` subcommand to the provided root command.
// It prints detailed build info.
func AttachCobraVersionCommand(root *cobra.Command) {
	// Subcommand: `version`.
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), Full())
		},
	})
}
