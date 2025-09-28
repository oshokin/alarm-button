package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds connection parameters shared by the alarm binaries.
type Config struct {
	// ServerAddress is the gRPC server address for alarm service connections.
	ServerAddress string `yaml:"server_addr"`
	// ServerUpdateFolder is the URL where update artifacts are hosted.
	ServerUpdateFolder string `yaml:"update_folder"`
	// StateFile is the path to the JSON file storing alarm state.
	StateFile string `yaml:"state_file"`
	// Timeout is the duration for network operations and RPC calls.
	Timeout time.Duration `yaml:"timeout"`
	// UpdateType is set at runtime by the updater to pick a role-specific
	// file set from the update manifest. It is not persisted to YAML.
	UpdateType string `yaml:"-"`
}

const (
	// DefaultConfigFilename is the default filename for connection settings.
	DefaultConfigFilename = "alarm-button-settings.yaml"

	// DefaultStateFilename is the default filename for alarm state JSON.
	DefaultStateFilename = "alarm-button-state.json"

	// DefaultTimeout is the default duration for network operations.
	DefaultTimeout = 5 * time.Second

	// DefaultFilePermissions is the default file permission for config files.
	DefaultFilePermissions = 0o600
)

var (
	// errConfigIsNotSet is returned when a nil configuration is provided.
	errConfigIsNotSet = errors.New("configuration is not set")
	// errServerSocketRequired is returned when server address is missing.
	errServerSocketRequired = errors.New("server address must be provided")
)

// Load reads configuration from the provided path and validates essential fields.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigFilename
	}

	contents, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read settings: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(contents, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal settings: %w", err)
	}

	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Save writes Settings to the provided path.
func Save(path string, cfg *Config) error {
	if cfg == nil {
		return errConfigIsNotSet
	}

	if path == "" {
		path = DefaultConfigFilename
	}

	if err := Validate(cfg); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	// Restrict permissions.
	if err := os.WriteFile(filepath.Clean(path), data, DefaultFilePermissions); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}

// Validate checks the provided settings for required fields and formatting.
func Validate(settings *Config) error {
	if settings.ServerAddress == "" {
		return errServerSocketRequired
	}

	if _, err := net.ResolveTCPAddr("tcp", settings.ServerAddress); err != nil {
		return fmt.Errorf("invalid server socket: %w", err)
	}

	// Set default timeout if not specified
	if settings.Timeout <= 0 {
		settings.Timeout = DefaultTimeout
	}

	// Set default state file if not specified
	if settings.StateFile == "" {
		settings.StateFile = DefaultStateFilename
	}

	if settings.ServerUpdateFolder == "" {
		return nil
	}

	if _, err := url.ParseRequestURI(settings.ServerUpdateFolder); err != nil {
		return fmt.Errorf("invalid update folder URI: %w", err)
	}

	return nil
}
