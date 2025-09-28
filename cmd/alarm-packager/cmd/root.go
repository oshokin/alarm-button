package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/packager"
	"github.com/oshokin/alarm-button/internal/version"
)

var (
	// configPath to the configuration YAML file.
	configPath string

	// rootCmd represents the base command for preparing update metadata.
	rootCmd = &cobra.Command{
		Use:   "alarm-packager [server-socket] [update-folder]",
		Short: "Prepare update metadata for distribution",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			// Setup graceful shutdown handling.
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			options := &packager.Options{
				ConfigPath:    configPath,
				ServerAddress: args[0],
				UpdateFolder:  args[1],
			}

			return packager.Run(ctx, options)
		},
	}
)

// Execute runs the alarm-packager CLI and exits with non-zero status on error.
func Execute() {
	version.AttachCobraVersionCommand(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

//nolint:gochecknoinits // Required by Cobra CLI framework architecture.
func init() {
	// Setup command flags with consistent naming and descriptions.
	rootCmd.Flags().StringVarP(&configPath, "config", "c", config.DefaultConfigFilename, "path to configuration file")
}
