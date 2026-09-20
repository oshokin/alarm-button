package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/updater"
)

//nolint:cyclop,gocognit // Integration fixture intentionally performs end-to-end manifest flow in one scenario.
func TestUpdater_ConfigOnlyUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based integration fixture is unix-only")
	}

	dir := t.TempDir()
	prevWD, err := os.Getwd()
	require.NoError(t, err)

	t.Chdir(dir)
	t.Cleanup(func() { t.Chdir(prevWD) })
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	specs := updater.RoleSpecsForPlatform(runtime.GOOS)
	spec := specs[updater.RoleClient]

	versionScript := []byte(
		"#!/bin/sh\nif [ \"$1\" = \"version\" ] && [ \"$2\" = \"--short\" ]; then\n  echo 1.0.0\n  exit 0\nfi\necho $$ > alarm-checker.pid\ntrap 'rm -f alarm-checker.pid' EXIT\nsleep 30\n",
	)

	t.Cleanup(func() {
		pidBytes, readErr := os.ReadFile(spec.PIDFile)
		if readErr != nil {
			return
		}

		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
		if parseErr != nil {
			return
		}

		process, findErr := os.FindProcess(pid)
		if findErr != nil {
			return
		}

		killErr := process.Kill()
		if killErr != nil {
			t.Logf("cleanup: failed to stop checker fixture process %d: %v", pid, killErr)
		}
	})

	for _, name := range spec.Files {
		content := []byte(name + "-binary")
		perm := os.FileMode(0o644)

		if name == spec.Executable {
			content = versionScript
			perm = 0o755
		}

		require.NoError(t, os.WriteFile(name, content, perm))
	}

	configArtifactName := updater.ConfigArtifactName(updater.RoleClient)

	var (
		manifestBytes []byte
		signatureText string
		desiredCfg    []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + updater.VersionFilename:
			_, writeErr := w.Write(manifestBytes)
			if writeErr != nil {
				t.Errorf("write manifest response: %v", writeErr)
			}
		case "/" + updater.SignatureFilename:
			_, writeErr := w.Write([]byte(signatureText))
			if writeErr != nil {
				t.Errorf("write signature response: %v", writeErr)
			}
		case "/" + configArtifactName:
			_, writeErr := w.Write(desiredCfg)
			if writeErr != nil {
				t.Errorf("write config response: %v", writeErr)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	require.NoError(t, os.WriteFile(config.DefaultConfigFilename, []byte(`
server_addr: 127.0.0.1:50051
listen_addr: 127.0.0.1:50051
update_folder: `+server.URL+`
timeout: 5s
`), 0o600))

	desiredCfg = []byte(`
server_addr: 127.0.0.1:50051
listen_addr: 127.0.0.1:50051
update_folder: ` + server.URL + `
timeout: 10s
`)

	artifactChecksums := make(map[string]*updater.Artifact, len(spec.Files))
	for _, name := range spec.Files {
		checksum, checksumErr := checksumFileBase64(name)
		require.NoError(t, checksumErr)

		info, statErr := os.Stat(name)
		require.NoError(t, statErr)

		artifactChecksums[name] = &updater.Artifact{
			Kind:   updater.ArtifactKindExecutable,
			Size:   info.Size(),
			SHA512: checksum,
		}
	}

	configHash := sha512.Sum512(desiredCfg)
	configChecksum := base64.StdEncoding.EncodeToString(configHash[:])

	pubKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	const testKeyID = "test-key"

	manifest := &updater.Manifest{
		SchemaVersion: updater.CurrentManifestSchema,
		Version:       "1.0.0",
		Platform: updater.Platform{
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
		},
		Signing: updater.Signing{
			KeyID: testKeyID,
		},
		Artifacts: artifactChecksums,
		Configuration: map[updater.Role]*updater.ConfigArtifact{
			updater.RoleClient: {
				Revision: 1,
				Artifact: configArtifactName,
				Size:     int64(len(desiredCfg)),
				SHA512:   configChecksum,
			},
		},
	}

	manifestBytes, err = updater.MarshalManifest(manifest)
	require.NoError(t, err)

	signature := ed25519.Sign(privateKey, manifestBytes)
	signatureText = base64.StdEncoding.EncodeToString(signature) + "\n"

	err = updater.Run(context.Background(), &updater.Options{
		ConfigPath:          config.DefaultConfigFilename,
		UpdateType:          string(updater.RoleClient),
		InstallRoot:         dir,
		TrustedManifestKeys: map[string]ed25519.PublicKey{testKeyID: pubKey},
	})
	require.NoError(t, err)

	gotConfig, err := os.ReadFile(config.DefaultConfigFilename)
	require.NoError(t, err)
	require.Equal(t, desiredCfg, gotConfig)

	stateBytes, err := os.ReadFile(updater.UpdateStateFilename)
	require.NoError(t, err)

	var state struct {
		ConfigRevision uint64 `json:"config_revision"`
	}
	require.NoError(t, json.Unmarshal(stateBytes, &state))
	require.Equal(t, uint64(1), state.ConfigRevision)
}

// checksumFileBase64 computes base64-encoded SHA-512 checksum for fixture files.
func checksumFileBase64(path string) (string, error) {
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}

	sum := sha512.Sum512(content)

	return base64.StdEncoding.EncodeToString(sum[:]), nil
}
