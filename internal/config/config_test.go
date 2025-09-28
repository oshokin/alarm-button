package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidate checks required fields and format validations for Settings.
func TestValidate(t *testing.T) {
	t.Parallel()

	// Missing socket.
	settings := new(Config)

	err := Validate(settings)
	require.Error(t, err)

	// Bad socket.
	settings = &Config{
		ServerAddress: "bad:address",
	}

	err = Validate(settings)
	require.Error(t, err)

	// Okay with update folder.
	settings = &Config{
		ServerAddress:      "127.0.0.1:0",
		ServerUpdateFolder: "https://example.com/x",
	}

	err = Validate(settings)
	require.NoError(t, err)
}

// TestSaveLoadRoundtrip ensures settings are persisted and loaded back correctly.
func TestSaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")

	settings := &Config{
		ServerAddress:      "127.0.0.1:50051",
		ServerUpdateFolder: "https://updates.local/",
	}

	require.NoError(t, Save(path, settings))

	loaded, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, settings.ServerAddress, loaded.ServerAddress)
	require.Equal(t, settings.ServerUpdateFolder, loaded.ServerUpdateFolder)

	// File exists.
	_, err = os.Stat(path)
	require.NoError(t, err)
}
