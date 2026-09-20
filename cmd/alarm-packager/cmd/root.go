package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/service/packager"
	"github.com/oshokin/alarm-button/internal/version"
)

// CLI globals hold packager flags and root command wiring.
var (
	inputDir   string
	outputDir  string
	releaseVer string
	privateKey string
	targetGOOS string
	targetARCH string

	clientConfig         string
	clientConfigRevision uint64
	serverConfig         string
	serverConfigRevision uint64
	signingKeyID         string

	rootCmd = &cobra.Command{
		Use:   "alarm-packager",
		Short: "Build signed update manifest and artifacts",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return packager.Run(&packager.Options{
				InputDir:             inputDir,
				OutputDir:            outputDir,
				Version:              releaseVer,
				SigningKey:           privateKey,
				GOOS:                 targetGOOS,
				GOARCH:               targetARCH,
				ClientConfig:         clientConfig,
				ClientConfigRevision: clientConfigRevision,
				ServerConfig:         serverConfig,
				ServerConfigRevision: serverConfigRevision,
				KeyID:                signingKeyID,
			})
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
	rootCmd.AddCommand(newKeygenCommand())

	rootCmd.Flags().StringVar(&inputDir, "input-dir", "", "directory with built binaries for target platform")
	rootCmd.Flags().StringVar(&outputDir, "output-dir", "", "directory to place signed manifest and artifacts")
	rootCmd.Flags().StringVar(&releaseVer, "version", "", "release semantic version (e.g. 1.8.0)")
	rootCmd.Flags().StringVar(&privateKey, "private-key", "", "path to Ed25519 private key in PKCS8 PEM")
	rootCmd.Flags().StringVar(&targetGOOS, "goos", "", "target GOOS (default current GOOS)")
	rootCmd.Flags().StringVar(&targetARCH, "goarch", "", "target GOARCH (default current GOARCH)")
	rootCmd.Flags().
		StringVar(&clientConfig, "client-config", "", "path to client config YAML for centralized deployment")
	rootCmd.Flags().Uint64Var(&clientConfigRevision, "client-config-revision", 0, "client config revision")
	rootCmd.Flags().
		StringVar(&serverConfig, "server-config", "", "path to server config YAML for centralized deployment")
	rootCmd.Flags().Uint64Var(&serverConfigRevision, "server-config-revision", 0, "server config revision")
	rootCmd.Flags().StringVar(&signingKeyID, "key-id", "", "manifest signing key identifier")

	mustMarkFlagRequired("output-dir")
	mustMarkFlagRequired("version")
	mustMarkFlagRequired("private-key")
	mustMarkFlagRequired("key-id")
}

// mustMarkFlagRequired marks flag required and panics on Cobra wiring errors.
func mustMarkFlagRequired(name string) {
	if err := rootCmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}
