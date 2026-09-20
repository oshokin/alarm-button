package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidate verifies core config validation rules.
func TestValidate(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	applyDefaults(cfg)
	require.Error(t, Validate(cfg))

	cfg = &Config{
		ServerAddress: "bad:address",
		ListenAddress: "127.0.0.1:50051",
		Timeout:       DefaultTimeout,
	}
	require.Error(t, Validate(cfg))

	cfg = &Config{
		ServerAddress:      "127.0.0.1:0",
		ListenAddress:      "127.0.0.1:50051",
		ServerUpdateFolder: "https://example.com/x",
		Timeout:            DefaultTimeout,
	}
	require.NoError(t, Validate(cfg))
}

// TestValidate_InsecureUpdateURLPolicy verifies insecure update URL policy behavior.
func TestValidate_InsecureUpdateURLPolicy(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		ServerAddress:      "127.0.0.1:0",
		ListenAddress:      "127.0.0.1:50051",
		ServerUpdateFolder: "http://updates.example.com",
		Timeout:            DefaultTimeout,
	}

	require.Error(t, Validate(cfg))

	cfg.AllowInsecureUpdateURL = true
	require.NoError(t, Validate(cfg))
}

// TestSaveLoadRoundtrip verifies config serialization roundtrip.
func TestSaveLoadRoundtrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "settings.yaml")

	cfg := &Config{
		ServerAddress:      "127.0.0.1:50051",
		ServerUpdateFolder: "https://updates.local/",
		ListenAddress:      "127.0.0.1:50051",
	}

	require.NoError(t, Save(path, cfg))

	loaded, err := Load(path)
	require.NoError(t, err)
	require.Equal(t, cfg.ServerAddress, loaded.ServerAddress)
	require.Equal(t, cfg.ServerUpdateFolder, loaded.ServerUpdateFolder)
	require.Equal(t, cfg.ListenAddress, loaded.ListenAddress)

	_, err = os.Stat(path)
	require.NoError(t, err)
}

// TestParse_UnknownFieldRejected verifies strict YAML field decoding.
func TestParse_UnknownFieldRejected(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte(`
servr_addr: 127.0.0.1:50051
listen_addr: 127.0.0.1:50051
timeout: 5s
`))
	require.Error(t, err)
}
