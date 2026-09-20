package config

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"

	"github.com/oshokin/alarm-button/internal/fsutil"
)

// Config holds connection parameters shared by the alarm binaries.
type Config struct {
	// ServerAddress is the gRPC server address for alarm service connections.
	ServerAddress string `yaml:"server_addr"`
	// ListenAddress is the explicit bind address used by alarm-server.
	ListenAddress string `yaml:"listen_addr,omitempty"`
	// ServerUpdateFolder is the URL where update artifacts are hosted.
	ServerUpdateFolder string `yaml:"update_folder,omitempty"`
	// StateFile is the path to the JSON file storing alarm state.
	StateFile string `yaml:"state_file,omitempty"`
	// Timeout is the duration for network operations and RPC calls.
	Timeout time.Duration `yaml:"timeout,omitempty"`
	// TLS configures transport security.
	TLS TLSConfig `yaml:"tls,omitempty"`
	// AllowInsecureRemote allows non-loopback gRPC client connections without TLS.
	AllowInsecureRemote bool `yaml:"allow_insecure_remote,omitempty"`
	// AllowInsecureUpdateURL allows non-HTTPS update URLs.
	AllowInsecureUpdateURL bool `yaml:"allow_insecure_update_url,omitempty"`
	// UpdateType is set at runtime by the updater to pick a role-specific
	// file set from the update manifest. It is not persisted to YAML.
	UpdateType string `yaml:"-"`
}

// TLSConfig controls transport security for gRPC communication.
type TLSConfig struct {
	// Enabled toggles TLS transport for gRPC server/client.
	Enabled bool `yaml:"enabled"`

	// CAFile is optional CA bundle path for peer certificate verification.
	CAFile string `yaml:"ca_file,omitempty"`
	// CertFile is certificate path used for TLS server/client identity.
	CertFile string `yaml:"cert_file,omitempty"`
	// KeyFile is private key path paired with CertFile.
	KeyFile string `yaml:"key_file,omitempty"`

	// ServerName overrides TLS server name for client verification.
	ServerName string `yaml:"server_name,omitempty"`

	// RequireClientCertificate enforces mutual TLS on server side.
	RequireClientCertificate bool `yaml:"require_client_certificate,omitempty"`
}

// Default configuration constants used across binaries.
const (
	// DefaultConfigFilename is the default filename for connection settings.
	DefaultConfigFilename = "alarm-button-settings.yaml"

	// DefaultListenAddress is the default gRPC bind address.
	DefaultListenAddress = "127.0.0.1:50051"

	// DefaultStateFilename is the default filename for alarm state JSON.
	DefaultStateFilename = "alarm-button-state.json"

	// DefaultTimeout is the default duration for network operations.
	DefaultTimeout = 5 * time.Second

	// DefaultFilePermissions is the default file permission for config files.
	DefaultFilePermissions = 0o600
)

// Validation and policy errors for configuration parsing and checks.
var (
	// errConfigIsNotSet is returned when a nil configuration is provided.
	errConfigIsNotSet = errors.New("configuration is not set")
	// errServerAddressRequired is returned when server address is missing.
	errServerAddressRequired = errors.New("server address must be provided")
	// errListenAddressRequired is returned when listen address is missing.
	errListenAddressRequired = errors.New("listen address must be provided")
	// errTimeoutMustBePositive is returned when timeout is zero or negative.
	errTimeoutMustBePositive = errors.New("timeout must be positive")
	// errTLSFieldsSetWhileDisabled is returned when tls.enabled=false but TLS fields are set.
	errTLSFieldsSetWhileDisabled = errors.New("tls fields are set but tls.enabled is false")
	// errTLSCertKeyPairRequired is returned when cert/key pairing is incomplete.
	errTLSCertKeyPairRequired = errors.New("tls cert_file and key_file must be set together")
	// errTLSCARequiredForClientCert is returned when client cert verification is enabled without CA.
	errTLSCARequiredForClientCert = errors.New("tls ca_file is required when require_client_certificate=true")
	// errInsecureUpdateURLNotAllowed is returned when insecure update URL is disallowed.
	errInsecureUpdateURLNotAllowed = errors.New(
		"insecure update URL requires localhost host or allow_insecure_update_url=true",
	)
	// errUnsupportedUpdateURLScheme is returned when update URL scheme is not http/https.
	errUnsupportedUpdateURLScheme = errors.New("unsupported update URL scheme")
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

	return Parse(contents)
}

// Parse decodes and validates configuration from raw YAML bytes.
func Parse(data []byte) (*Config, error) {
	var cfg Config

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode settings: %w", err)
	}

	applyDefaults(&cfg)

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

	copyCfg := *cfg
	applyDefaults(&copyCfg)

	if err := Validate(&copyCfg); err != nil {
		return err
	}

	data, err := yaml.Marshal(&copyCfg)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	// Restrict permissions.
	if err = fsutil.WriteFileAtomic(filepath.Clean(path), data, DefaultFilePermissions); err != nil {
		return fmt.Errorf("write settings: %w", err)
	}

	return nil
}

// Validate checks the provided settings for required fields and formatting.
func Validate(cfg *Config) error {
	if cfg == nil {
		return errConfigIsNotSet
	}

	if cfg.ServerAddress == "" {
		return errServerAddressRequired
	}

	if _, err := net.ResolveTCPAddr("tcp", cfg.ServerAddress); err != nil {
		return fmt.Errorf("invalid server address: %w", err)
	}

	if cfg.ListenAddress == "" {
		return errListenAddressRequired
	}

	if _, err := net.ResolveTCPAddr("tcp", cfg.ListenAddress); err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}

	if cfg.Timeout <= 0 {
		return fmt.Errorf("%w: %s", errTimeoutMustBePositive, cfg.Timeout)
	}

	if err := validateTLSConfig(&cfg.TLS); err != nil {
		return err
	}

	if cfg.ServerUpdateFolder == "" {
		return nil
	}

	return validateUpdateSourceURL(cfg.ServerUpdateFolder, cfg.AllowInsecureUpdateURL)
}

// validateTLSConfig validates TLS constraints for enabled/disabled modes.
func validateTLSConfig(tlsCfg *TLSConfig) error {
	if tlsCfg == nil {
		return nil
	}

	if !tlsCfg.Enabled {
		return validateTLSDisabled(tlsCfg)
	}

	if (tlsCfg.CertFile == "") != (tlsCfg.KeyFile == "") {
		return errTLSCertKeyPairRequired
	}

	if tlsCfg.RequireClientCertificate && tlsCfg.CAFile == "" {
		return errTLSCARequiredForClientCert
	}

	return nil
}

// validateTLSDisabled rejects non-empty TLS fields when tls.enabled=false.
func validateTLSDisabled(tlsCfg *TLSConfig) error {
	hasTLSFields := tlsCfg.CAFile != "" ||
		tlsCfg.CertFile != "" ||
		tlsCfg.KeyFile != "" ||
		tlsCfg.ServerName != "" ||
		tlsCfg.RequireClientCertificate
	if hasTLSFields {
		return errTLSFieldsSetWhileDisabled
	}

	return nil
}

// validateUpdateSourceURL validates update repository URL and insecure policy.
func validateUpdateSourceURL(rawURL string, allowInsecure bool) error {
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid update folder URI: %w", err)
	}

	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		if allowInsecure || isLoopbackHost(parsed.Hostname()) {
			return nil
		}

		return errInsecureUpdateURLNotAllowed
	default:
		return fmt.Errorf("%w %q", errUnsupportedUpdateURLScheme, parsed.Scheme)
	}
}

// isLoopbackHost reports whether host is localhost or loopback IP.
func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// ValidateLocalEnvironment validates local prerequisites for parsed configuration.
func (cfg *Config) ValidateLocalEnvironment() error {
	if cfg == nil {
		return errConfigIsNotSet
	}

	if !cfg.TLS.Enabled {
		return nil
	}

	for _, file := range []string{cfg.TLS.CAFile, cfg.TLS.CertFile, cfg.TLS.KeyFile} {
		if file == "" {
			continue
		}

		if _, err := os.Stat(filepath.Clean(file)); err != nil {
			return fmt.Errorf("tls file %q: %w", file, err)
		}
	}

	return nil
}

// applyDefaults applies default values for optional config fields.
func applyDefaults(cfg *Config) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}

	if cfg.StateFile == "" {
		cfg.StateFile = DefaultStateFilename
	}

	if cfg.ListenAddress == "" {
		cfg.ListenAddress = DefaultListenAddress
	}
}
