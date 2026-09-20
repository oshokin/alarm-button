package packager

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/oshokin/alarm-button/internal/service/updater"
)

// TestPrepareRunInputs_RejectsMissingContent verifies package content requirement.
func TestPrepareRunInputs_RejectsMissingContent(t *testing.T) {
	t.Parallel()

	keyPath := writeSigningKey(t)
	_, _, err := prepareRunInputs(&Options{
		OutputDir:  t.TempDir(),
		Version:    "1.2.3",
		SigningKey: keyPath,
		KeyID:      "test-key",
	})
	require.ErrorIs(t, err, errPackageContentRequired)
}

// TestPrepareRunInputs_RejectsMissingKeyID verifies mandatory key id validation.
func TestPrepareRunInputs_RejectsMissingKeyID(t *testing.T) {
	t.Parallel()

	keyPath := writeSigningKey(t)

	err := validateRequiredRunFields(&Options{
		OutputDir:  t.TempDir(),
		Version:    "1.2.3",
		SigningKey: keyPath,
	})
	require.ErrorIs(t, err, errKeyIDRequired)
}

// TestPrepareRunInputs_BinaryReleaseRequiresRoleConfigs verifies binary publish contract.
func TestPrepareRunInputs_BinaryReleaseRequiresRoleConfigs(t *testing.T) {
	t.Parallel()

	keyPath := writeSigningKey(t)
	_, _, err := prepareRunInputs(&Options{
		InputDir:   t.TempDir(),
		OutputDir:  t.TempDir(),
		Version:    "1.2.3",
		SigningKey: keyPath,
		KeyID:      "test-key",
	})
	require.ErrorIs(t, err, errClientConfigRequired)
}

// TestRun_ConfigOnlyRelease verifies config-only package emission path.
func TestRun_ConfigOnlyRelease(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outputDir := filepath.Join(dir, "output")
	clientCfgPath := filepath.Join(dir, "client.yaml")
	keyPath := writeSigningKey(t)

	require.NoError(t, os.WriteFile(clientCfgPath, []byte(`
server_addr: 127.0.0.1:50051
listen_addr: 127.0.0.1:50051
update_folder: https://updates.example.com/client
timeout: 5s
`), 0o600))

	err := Run(&Options{
		OutputDir:            outputDir,
		Version:              "1.2.3",
		SigningKey:           keyPath,
		KeyID:                "test-key",
		GOOS:                 runtime.GOOS,
		GOARCH:               runtime.GOARCH,
		ClientConfig:         clientCfgPath,
		ClientConfigRevision: 7,
	})
	require.NoError(t, err)

	manifestBytes, err := os.ReadFile(filepath.Join(outputDir, updater.VersionFilename))
	require.NoError(t, err)

	var manifest updater.Manifest
	require.NoError(t, yaml.Unmarshal(manifestBytes, &manifest))
	require.Empty(t, manifest.Artifacts)
	require.Contains(t, manifest.Configuration, updater.RoleClient)

	clientCfgMeta := manifest.Configuration[updater.RoleClient]
	require.Equal(t, updater.ConfigArtifactName(updater.RoleClient), clientCfgMeta.Artifact)
	require.Equal(t, uint64(7), clientCfgMeta.Revision)
}

// writeSigningKey creates temporary PKCS8 Ed25519 private key fixture.
func writeSigningKey(t *testing.T) string {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "signing-key.pem")
	require.NoError(
		t,
		os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0o600),
	)

	return path
}
