package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/updater"
	"github.com/oshokin/alarm-button/internal/version"
)

var (
	// configPath to the configuration YAML file.
	configPath string

	// rootCmd represents the base command for downloading and applying updates.
	rootCmd = &cobra.Command{
		Use:       "alarm-updater [client|server]",
		Short:     "Download and apply updates from the server",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"client", "server"},
		RunE: func(_ *cobra.Command, args []string) error {
			// Setup graceful shutdown handling.
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			options := &updater.Options{
				ConfigPath: configPath,
				UpdateType: args[0],
			}

			return updater.Run(ctx, options)
		},
	}
)

// Execute runs the alarm-updater CLI and exits with non-zero status on error.
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
