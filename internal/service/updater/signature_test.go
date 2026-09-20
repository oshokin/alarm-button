package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// TestVerifyManifest_ValidSignature verifies successful signature validation.
func TestVerifyManifest_ValidSignature(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifest := []byte("schema_version: 2\n")
	signature := ed25519.Sign(priv, manifest)
	signatureText := []byte(base64.StdEncoding.EncodeToString(signature))

	err = verifyManifestWithKeys(manifest, signatureText, map[string]ed25519.PublicKey{"test": pub})
	require.NoError(t, err)
}

// TestVerifyManifest_InvalidSignature verifies malformed signature rejection.
func TestVerifyManifest_InvalidSignature(t *testing.T) {
	t.Parallel()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	err = verifyManifestWithKeys(
		[]byte("schema_version: 2\n"),
		[]byte(base64.StdEncoding.EncodeToString([]byte("not-a-signature"))),
		map[string]ed25519.PublicKey{"test": pub},
	)
	require.Error(t, err)
}

// TestVerifyManifest_ModifiedManifestRejected verifies tampered payload rejection.
func TestVerifyManifest_ModifiedManifestRejected(t *testing.T) {
	t.Parallel()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifest := []byte("version: 1.0.0\n")
	signature := ed25519.Sign(priv, manifest)

	tampered := []byte("version: 2.0.0\n")
	err = verifyManifestWithKeys(
		tampered,
		[]byte(base64.StdEncoding.EncodeToString(signature)),
		map[string]ed25519.PublicKey{"test": pub},
	)
	require.Error(t, err)
}

// TestVerifyManifest_KeyIDMustMatchSigner verifies strict key_id signer binding.
func TestVerifyManifest_KeyIDMustMatchSigner(t *testing.T) {
	t.Parallel()

	pubA, privA, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pubB, privB, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifestBytes, err := yaml.Marshal(&Manifest{
		Signing: Signing{KeyID: "key-a"},
	})
	require.NoError(t, err)

	signatureFromA := ed25519.Sign(privA, manifestBytes)
	err = verifyManifestWithKeys(
		manifestBytes,
		[]byte(base64.StdEncoding.EncodeToString(signatureFromA)),
		map[string]ed25519.PublicKey{
			"key-a": pubA,
			"key-b": pubB,
		},
	)
	require.NoError(t, err)

	signatureFromB := ed25519.Sign(privB, manifestBytes)
	err = verifyManifestWithKeys(
		manifestBytes,
		[]byte(base64.StdEncoding.EncodeToString(signatureFromB)),
		map[string]ed25519.PublicKey{
			"key-a": pubA,
			"key-b": pubB,
		},
	)
	require.ErrorIs(t, err, errInvalidManifestSignature)
}

// TestVerifyManifest_UnknownKeyIDRejected verifies unknown key id policy.
func TestVerifyManifest_UnknownKeyIDRejected(t *testing.T) {
	t.Parallel()

	pubKnown, privKnown, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifestBytes, err := yaml.Marshal(&Manifest{
		Signing: Signing{KeyID: "unknown-key"},
	})
	require.NoError(t, err)

	signature := ed25519.Sign(privKnown, manifestBytes)
	err = verifyManifestWithKeys(
		manifestBytes,
		[]byte(base64.StdEncoding.EncodeToString(signature)),
		map[string]ed25519.PublicKey{"known-key": pubKnown},
	)
	require.ErrorIs(t, err, errManifestKeyIDUnknown)
}

// verifyManifestWithKeys verifies manifest signature using test-provided trusted keys.
func verifyManifestWithKeys(
	manifest []byte,
	signatureText []byte,
	trustedKeys map[string]ed25519.PublicKey,
) error {
	testRunner := &runner{trustedManifestKeys: trustedKeys}

	return testRunner.verifyManifest(manifest, signatureText)
}
