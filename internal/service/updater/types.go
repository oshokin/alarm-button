package updater

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
)

// Updater manifest/state filenames, schema version, and platform constants.
const (
	goosWindows = "windows"

	// VersionFilename stores the signed update manifest.
	VersionFilename = "alarm-button-version.yaml"
	// SignatureFilename stores the base64 manifest signature.
	SignatureFilename = "alarm-button-version.yaml.sig"
	// UpdateStateFilename stores local updater state.
	UpdateStateFilename = "alarm-button-update-state.json"
	// UpdateLockFilename serializes updater runs.
	UpdateLockFilename = "alarm-button-update.lock"

	// CurrentManifestSchema is the accepted manifest schema.
	CurrentManifestSchema = 2
	// maxManifestSize prevents unbounded manifest downloads.
	maxManifestSize = 1 << 20 // 1 MiB
)

// Updater manifest/state validation errors.
var (
	errUnsupportedRole          = errors.New("unsupported role")
	errUnsafeArtifactName       = errors.New("unsafe artifact name")
	errManifestTooLarge         = errors.New("manifest is too large")
	errInvalidManifestSignature = errors.New("invalid manifest signature")
	errConfigRevisionReused     = errors.New("configuration revision reused with different content")
	errConfigRollbackRejected   = errors.New("configuration rollback rejected")
	errBinaryDowngradeRejected  = errors.New("binary downgrade rejected")
	errNoTrustedManifestKey     = errors.New("no trusted manifest key")
	errConfigArtifactMissing    = errors.New("configuration artifact is missing")
	errConfigTargetPathMissing  = errors.New("configuration target path is missing")
	errUnsupportedTargetKind    = errors.New("unsupported artifact kind")
)

// Role describes updater deployment role.
type Role string

// Role identifiers supported by updater workflows.
const (
	RoleClient Role = "client"
	RoleServer Role = "server"
)

// ArtifactKind describes a published artifact type.
type ArtifactKind string

// Artifact and role-local file naming constants.
const (
	ArtifactKindExecutable ArtifactKind = "executable"
	ArtifactKindConfig     ArtifactKind = "config"

	configArtifactClientName = "alarm-button-settings-client.yaml"
	configArtifactServerName = "alarm-button-settings-server.yaml"
	pidFileClientName        = "alarm-checker.pid"
	pidFileServerName        = "alarm-server.pid"
)

// Options are updater CLI inputs.
type Options struct {
	// ConfigPath is path to local updater configuration YAML.
	ConfigPath string
	// UpdateType defines updater role mode (client/server).
	UpdateType string

	// AllowDowngrade allows applying binary versions lower than local.
	AllowDowngrade bool
	// AllowConfigRollback allows applying configuration revision rollback.
	AllowConfigRollback bool

	// InstallRoot overrides executable/config/state root, primarily for tests.
	InstallRoot string
	// TrustedManifestKeys overrides embedded trusted roots, primarily for tests.
	TrustedManifestKeys map[string]ed25519.PublicKey
}

// RoleSpec defines local trusted artifact mapping.
type RoleSpec struct {
	// Name is role identifier for the specification.
	Name Role
	// Files lists role-scoped executable artifact names.
	Files []string
	// Executable is the managed process executable file name.
	Executable string
	// PIDFile is PID file name written by managed process.
	PIDFile string
	// ConfigArtifact is expected repository config artifact file name.
	ConfigArtifact string
}

// Manifest is the signed desired-state document.
type Manifest struct {
	// SchemaVersion is manifest format version.
	SchemaVersion int `yaml:"schema_version"`
	// Version is desired application semantic version.
	Version string `yaml:"version"`

	// Platform limits manifest to GOOS/GOARCH target.
	Platform Platform `yaml:"platform"`

	// Signing stores signature metadata such as key identifier.
	Signing Signing `yaml:"signing,omitempty"`

	// Artifacts maps executable artifact file name to metadata.
	Artifacts map[string]*Artifact `yaml:"artifacts"`

	// Configuration maps role to configuration artifact metadata.
	Configuration map[Role]*ConfigArtifact `yaml:"configuration,omitempty"`
}

// Signing carries manifest signing metadata.
type Signing struct {
	// KeyID selects trusted public key used to verify signature.
	KeyID string `yaml:"key_id,omitempty"`
}

// Platform narrows a manifest to one GOOS/GOARCH target.
type Platform struct {
	// GOOS is target operating system.
	GOOS string `yaml:"goos"`
	// GOARCH is target architecture.
	GOARCH string `yaml:"goarch"`
}

// Artifact metadata for executable artifacts.
type Artifact struct {
	// Kind is artifact category.
	Kind ArtifactKind `yaml:"kind"`
	// Size is expected artifact size in bytes.
	Size int64 `yaml:"size"`
	// SHA512 is expected base64-encoded SHA-512 checksum.
	SHA512 string `yaml:"sha512"`
}

// ConfigArtifact metadata for role-specific configuration deployment.
type ConfigArtifact struct {
	// Revision is monotonic configuration revision for rollback policy.
	Revision uint64 `yaml:"revision"`
	// Artifact is repository file name containing configuration.
	Artifact string `yaml:"artifact"`
	// Size is expected configuration artifact size in bytes.
	Size int64 `yaml:"size"`
	// SHA512 is expected base64-encoded SHA-512 checksum.
	SHA512 string `yaml:"sha512"`
}

// UpdateState stores last successfully applied desired state.
type UpdateState struct {
	// Role is updater role that wrote state.
	Role Role `json:"role"`
	// ApplicationVersion is last applied binary version for this role.
	ApplicationVersion string `json:"application_version"`

	// ConfigRevision is last applied configuration revision.
	ConfigRevision uint64 `json:"config_revision"`
	// ConfigSHA512 is checksum of last applied configuration.
	ConfigSHA512 string `json:"config_sha512"`
}

// ConfigArtifactName returns expected config artifact name for a role.
func ConfigArtifactName(role Role) string {
	return configArtifactName(role)
}

// RoleSpecsForPlatform returns trusted role specs for a target OS.
func RoleSpecsForPlatform(goos string) map[Role]*RoleSpec {
	specs := roleSpecs(goos)

	result := make(map[Role]*RoleSpec, len(specs))
	for role, spec := range specs {
		if spec == nil {
			continue
		}

		copied := *spec
		copied.Files = append([]string(nil), spec.Files...)
		result[role] = &copied
	}

	return result
}

// parseRole parses raw role text into updater role enum.
func parseRole(s string) (Role, error) {
	switch Role(s) {
	case RoleClient:
		return RoleClient, nil
	case RoleServer:
		return RoleServer, nil
	default:
		return "", fmt.Errorf("%s: %w", s, errUnsupportedRole)
	}
}

// executableExtension returns executable suffix for target OS.
func executableExtension(goos string) string {
	if goos == goosWindows {
		return ".exe"
	}

	return ""
}

// roleSpecs builds role-to-artifact mappings for target platform.
func roleSpecs(goos string) map[Role]*RoleSpec {
	ext := executableExtension(goos)

	return map[Role]*RoleSpec{
		RoleClient: {
			Name: RoleClient,
			Files: []string{
				"alarm-button-on" + ext,
				"alarm-checker" + ext,
				"alarm-updater" + ext,
			},
			Executable:     "alarm-checker" + ext,
			PIDFile:        pidFileClientName,
			ConfigArtifact: configArtifactClientName,
		},
		RoleServer: {
			Name: RoleServer,
			Files: []string{
				"alarm-button-off" + ext,
				"alarm-server" + ext,
				"alarm-updater" + ext,
			},
			Executable:     "alarm-server" + ext,
			PIDFile:        pidFileServerName,
			ConfigArtifact: configArtifactServerName,
		},
	}
}

// configArtifactName returns role-specific configuration artifact name.
func configArtifactName(role Role) string {
	switch role {
	case RoleClient:
		return configArtifactClientName
	case RoleServer:
		return configArtifactServerName
	default:
		return ""
	}
}

// targetPathForArtifact resolves installation target path for staged artifact.
func (r *runner) targetPathForArtifact(artifact *stagedArtifact) (string, error) {
	switch artifact.Kind {
	case ArtifactKindExecutable:
		if !filepath.IsLocal(artifact.Name) {
			return "", fmt.Errorf("%q: %w", artifact.Name, errUnsafeArtifactName)
		}

		return filepath.Join(filepath.Clean(r.installRoot), artifact.Name), nil

	case ArtifactKindConfig:
		if r.configPath == "" {
			return "", errConfigTargetPathMissing
		}

		return filepath.Clean(r.configPath), nil

	default:
		return "", fmt.Errorf("%w %q", errUnsupportedTargetKind, artifact.Kind)
	}
}

// localRoleSpec returns role specification for current runtime platform.
func localRoleSpec(role Role) (*RoleSpec, error) {
	spec, ok := roleSpecs(runtime.GOOS)[role]
	if !ok || spec == nil {
		return nil, fmt.Errorf("%s: %w", role, errUnsupportedRole)
	}

	copied := *spec
	copied.Files = append([]string(nil), spec.Files...)

	return &copied, nil
}
