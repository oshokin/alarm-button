package cmd

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/config"
	client "github.com/oshokin/alarm-button/internal/service/client"
	"github.com/oshokin/alarm-button/internal/version"
)

var (
	// cfgPath stores the configuration file path.
	cfgPath string
	// debug controls whether to skip shutdown for debugging.
	debug bool

	// rootCmd represents the base command for enabling alarm state.
	rootCmd = &cobra.Command{
		Use:   "alarm-button-on [server-address]",
		Short: "Enable alarm and shutdown this PC.",
		Long: `Activates the alarm system and shuts down the local machine.

Sends alarm enable requests to the server continuously until confirmation is received.
After successful activation, automatically shuts down this PC unless debug mode is enabled.
Server address can be provided as argument or loaded from configuration file.

This is typically used when leaving the office to activate security and shutdown the workstation.`,
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

			return client.Run(ctx, &client.Options{
				ConfigPath:    cfgPath,
				ServerAddress: serverAddress,
				DesiredState:  true,
				Debug:         debug,
			})
		},
	}
)

// Execute runs the alarm-button-on CLI and exits with non-zero status on error.
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

	// Hidden debug flag to skip shutdown for debugging.
	rootCmd.Flags().BoolVarP(&debug, "debug", "d", false, "skip shutdown for debugging")

	err := rootCmd.Flags().MarkHidden("debug")
	if err != nil {
		panic(err)
	}
}
