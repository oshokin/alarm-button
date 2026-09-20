package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/oshokin/alarm-button/internal/service/packager"
)

// newKeygenCommand builds "keygen" subcommand for update signing key bootstrap.
func newKeygenCommand() *cobra.Command {
	privateKeyPath := "update-signing-key.pem"
	keyID := packager.SuggestedKeyID()
	force := false

	command := &cobra.Command{
		Use:   "keygen",
		Short: "Generate update signing key pair",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			material, err := packager.GenerateSigningKey(privateKeyPath, keyID, force)
			if err != nil {
				return err
			}

			fmt.Printf("Private key saved to: %s\n", material.PrivateKeyPath)
			fmt.Printf("Recommended key ID: %s\n", material.KeyID)
			fmt.Printf("Trusted public key (base64): %s\n", material.PublicKeyBase64)

			return nil
		},
	}

	command.Flags().StringVar(
		&privateKeyPath,
		"private-key",
		privateKeyPath,
		"output path for private key in PKCS8 PEM",
	)
	command.Flags().StringVar(&keyID, "key-id", keyID, "manifest signing key identifier")
	command.Flags().BoolVar(&force, "force", false, "overwrite existing private key output file")

	return command
}
