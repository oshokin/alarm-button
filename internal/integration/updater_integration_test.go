package integration

import (
	"context"
	"crypto/sha512"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/updater"
)

// TestUpdater_Run_FetchesAndApplies serves a manifest and file over HTTP and verifies the updater downloads and applies before failing to start.
//
//nolint:funlen // Integration test requires comprehensive setup and verification.
func TestUpdater_Run_FetchesAndApplies(t *testing.T) {
	// Setup test directory and change working directory.
	dir := t.TempDir()
	prev, _ := os.Getwd()

	t.Chdir(dir)
	t.Cleanup(func() {
		t.Chdir(prev)
	})

	// Start real gRPC server for reachability check.
	addr := reservePort(t)
	statePath := filepath.Join(dir, "state.json")

	stop := startGRPC(t, addr, statePath)
	defer stop()

	// Prepare test file content and checksum for download.
	fileName := "dummy.bin"
	fileBody := []byte("dummy-contents")
	checksum := sha512.Sum512(fileBody)
	checksumB64 := base64.StdEncoding.EncodeToString(checksum[:])

	// Create update manifest with test file.
	manifest := &updater.Description{
		VersionNumber: "test-version",
		Files:         map[string]string{fileName: checksumB64},
		Roles:         map[string][]string{"client": {fileName}},
		Executables:   map[string]string{"client": "nonexistent-binary"},
	}

	manifestBytes, err := yaml.Marshal(manifest)
	require.NoError(t, err)

	// Setup HTTP server to serve manifest and files.
	mux := http.NewServeMux()
	mux.HandleFunc(
		"/"+updater.VersionFilename,
		func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(manifestBytes)
		},
	)

	mux.HandleFunc("/"+fileName, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(fileBody)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Create configuration file pointing to test HTTP server.
	cfgPath := filepath.Join(dir, config.DefaultConfigFilename)
	cfg := &config.Config{
		ServerAddress:      addr,
		ServerUpdateFolder: ts.URL,
	}

	require.NoError(t, config.Save(cfgPath, cfg))

	// Run updater - expect error due to missing executable after download.
	updaterOptions := &updater.Options{
		ConfigPath: cfgPath,
		UpdateType: "client",
	}

	err = updater.Run(context.Background(), updaterOptions)
	require.Error(t, err)

	// Verify file was downloaded/applied before executable start failure.
	_, err = os.Stat(fileName)
	require.NoError(t, err)
}
