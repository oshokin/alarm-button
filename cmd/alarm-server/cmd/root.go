package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/server"
	"github.com/oshokin/alarm-button/internal/version"
)

var (
	// configPath to the configuration YAML file.
	configPath string
	// stateFile path where alarm state is persisted.
	stateFile string

	// rootCmd represents the base command for running the gRPC server.
	rootCmd = &cobra.Command{
		Use:   "alarm-server [listen-address]",
		Short: "Run the alarm gRPC server and manage alarm state.",
		Long: `Starts the gRPC alarm server that manages alarm state and handles client requests.

The server listens on the specified address or uses settings from configuration file.
Only the port from ServerAddress config is used for listening (e.g., :8080).
Listen address can be provided as argument to override config (e.g., :9090, 0.0.0.0:8080).
Alarm state is persisted to JSON file for recovery across restarts.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			// Setup graceful shutdown handling.
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()

			// Use listen address argument if provided, otherwise rely on config.
			var listenAddress string
			if len(args) > 0 {
				listenAddress = args[0]
			}

			options := &server.Options{
				ConfigPath:    configPath,
				ListenAddress: listenAddress,
				StateFile:     stateFile,
			}

			return server.Run(ctx, options)
		},
	}
)

// Execute runs the alarm-server CLI and exits with non-zero status on error.
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
	rootCmd.Flags().
		StringVarP(&stateFile, "state-file", "s", config.DefaultStateFilename, "path to persist alarm state")
}
