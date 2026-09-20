package integration

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

	"github.com/oshokin/alarm-button/internal/service/packager"
	"github.com/oshokin/alarm-button/internal/service/updater"
)

// TestPackager_WritesSignedManifest verifies integration path for manifest and signature emission.
func TestPackager_WritesSignedManifest(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputDir := filepath.Join(dir, "input")
	outputDir := filepath.Join(dir, "output")

	require.NoError(t, os.MkdirAll(inputDir, 0o755))

	specs := updater.RoleSpecsForPlatform(runtime.GOOS)
	unique := map[string]struct{}{}

	for _, spec := range specs {
		for _, name := range spec.Files {
			unique[name] = struct{}{}
		}
	}

	for name := range unique {
		require.NoError(t, os.WriteFile(filepath.Join(inputDir, name), []byte(name), 0o755))
	}

	clientCfg := filepath.Join(dir, "client.yaml")
	serverCfg := filepath.Join(dir, "server.yaml")

	require.NoError(t, os.WriteFile(clientCfg, []byte(`
server_addr: 127.0.0.1:50051
listen_addr: 127.0.0.1:50051
update_folder: https://updates.example.com/client
timeout: 5s
`), 0o600))

	require.NoError(t, os.WriteFile(serverCfg, []byte(`
server_addr: 127.0.0.1:50051
listen_addr: 127.0.0.1:50051
update_folder: https://updates.example.com/server
timeout: 5s
`), 0o600))

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	keyPath := filepath.Join(dir, "signing-key.pem")
	pkcs8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), 0o600))

	err = packager.Run(&packager.Options{
		InputDir:             inputDir,
		OutputDir:            outputDir,
		Version:              "1.8.0",
		SigningKey:           keyPath,
		GOOS:                 runtime.GOOS,
		GOARCH:               runtime.GOARCH,
		ClientConfig:         clientCfg,
		ClientConfigRevision: 31,
		ServerConfig:         serverCfg,
		ServerConfigRevision: 19,
		KeyID:                "test-key",
	})
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(outputDir, updater.VersionFilename))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(outputDir, updater.SignatureFilename))
	require.NoError(t, err)
}
