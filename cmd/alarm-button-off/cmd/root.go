package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/client"
	"github.com/oshokin/alarm-button/internal/version"
)

var (
	// cfgPath stores the configuration file path.
	cfgPath string

	// rootCmd represents the base command for disabling alarm state.
	rootCmd = &cobra.Command{
		Use:   "alarm-button-off [server-address]",
		Short: "Disable alarm (no shutdown).",
		Long: `Deactivates the alarm system without affecting this PC.

Sends alarm disable requests to the server continuously until confirmation is received.
This command only changes the alarm state and never shuts down the local machine.
Server address can be provided as argument or loaded from configuration file.

This is used to safely disable security when arriving at the office.`,
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

			options := &client.Options{
				ConfigPath:    cfgPath,
				ServerAddress: serverAddress,
				DesiredState:  false,
			}

			return client.Run(ctx, options)
		},
	}
)

// Execute runs the alarm-button-off CLI and exits with non-zero status on error.
func Execute() {
	version.AttachCobraVersionCommand(rootCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

//nolint:gochecknoinits // Required by Cobra CLI framework architecture.
func init() {
	// Setup command flags with consistent naming and descriptions.
	rootCmd.Flags().StringVarP(&cfgPath, "config", "c", config.DefaultConfigFilename, "path to configuration file")
}
