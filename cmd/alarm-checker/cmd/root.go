package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/checker"
	"github.com/oshokin/alarm-button/internal/version"
)

var (
	// configPath stores the path to the configuration YAML file.
	configPath string
	// debug controls whether to skip shutdown when alarm is enabled.
	debug bool

	// rootCmd represents the base command for polling alarm state.
	rootCmd = &cobra.Command{
		Use:   "alarm-checker [server-address]",
		Short: "Monitor alarm and shutdown when activated.",
		Long: `Background service that monitors alarm state and shuts down PC when alarm is enabled.

Continuously polls the server at fixed 5-second intervals to check alarm status.
When alarm becomes active (enabled by any source), immediately shuts down this PC.
Uses timeout and server settings from configuration file, polling interval is fixed.
Server address can be provided as argument or loaded from configuration file.

This runs as a background service to automatically shutdown when security is activated.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// Setup graceful shutdown handling.
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			// Use server address argument if provided, otherwise rely on config.
			var serverAddress string
			if len(args) > 0 {
				serverAddress = args[0]
			}

			// Create checker options with server address override and debug flag.
			checkerOptions := &checker.Options{
				ConfigPath:    configPath,
				ServerAddress: serverAddress,
				Debug:         debug,
			}

			return checker.Run(ctx, checkerOptions)
		},
	}
)

// Execute runs the alarm-checker CLI and exits with non-zero status on error.
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

	// Hidden debug flag to skip shutdown for debugging.
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "skip shutdown for debugging")

	err := rootCmd.Flags().MarkHidden("debug")
	if err != nil {
		panic(err)
	}
}
