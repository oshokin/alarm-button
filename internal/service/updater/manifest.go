package updater

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/semver"
)

// Manifest validation and semantic version parsing errors.
var (
	errManifestIsNil            = errors.New("manifest is nil")
	errUnsupportedSchema        = errors.New("unsupported manifest schema")
	errManifestGOOSMismatch     = errors.New("manifest GOOS mismatch")
	errManifestGOARCHMismatch   = errors.New("manifest GOARCH mismatch")
	errUnexpectedArtifact       = errors.New("unexpected artifact")
	errUnsupportedArtifactKind  = errors.New("unsupported artifact kind")
	errArtifactSizeInvalid      = errors.New("artifact size must be positive")
	errUnexpectedConfigArtifact = errors.New("unexpected config artifact")
	errConfigRevisionInvalid    = errors.New("configuration revision must be positive")
	errConfigSizeInvalid        = errors.New("configuration size must be positive")
	errVersionIsEmpty           = errors.New("version is empty")
	errInvalidSemanticVersion   = errors.New("invalid semantic version")
	errInvalidSHA512Length      = errors.New("invalid sha512 length")
)

// Validate enforces manifest schema, platform, artifacts, and role config contract.
func (m *Manifest) Validate(spec *RoleSpec) error {
	if m == nil {
		return errManifestIsNil
	}

	err := m.validateHeader()
	if err != nil {
		return err
	}

	err = m.validateArtifacts()
	if err != nil {
		return err
	}

	return m.validateRoleConfiguration(spec)
}

// validateHeader validates top-level schema, platform, and version fields.
func (m *Manifest) validateHeader() error {
	if m.SchemaVersion != CurrentManifestSchema {
		return fmt.Errorf("%w: %d", errUnsupportedSchema, m.SchemaVersion)
	}

	normalizedVersion, err := normalizeSemVer(m.Version)
	if err != nil {
		return fmt.Errorf("invalid manifest version: %w", err)
	}

	m.Version = normalizedVersion

	if m.Platform.GOOS != runtime.GOOS {
		return fmt.Errorf("%w: got %q, want %q", errManifestGOOSMismatch, m.Platform.GOOS, runtime.GOOS)
	}

	if m.Platform.GOARCH != runtime.GOARCH {
		return fmt.Errorf("%w: got %q, want %q", errManifestGOARCHMismatch, m.Platform.GOARCH, runtime.GOARCH)
	}

	return nil
}

// validateArtifacts validates executable artifact allow-list and integrity metadata.
func (m *Manifest) validateArtifacts() error {
	allowedArtifacts := make(map[string]struct{}, 8)

	for _, roleSpec := range roleSpecs(runtime.GOOS) {
		if roleSpec == nil {
			continue
		}

		for _, name := range roleSpec.Files {
			allowedArtifacts[name] = struct{}{}
		}
	}

	for name, artifact := range m.Artifacts {
		err := m.validateArtifactMetadata(name, artifact, allowedArtifacts)
		if err != nil {
			return err
		}
	}

	return nil
}

// validateArtifactMetadata validates one executable artifact metadata entry.
func (m *Manifest) validateArtifactMetadata(
	name string,
	artifact *Artifact,
	allowedArtifacts map[string]struct{},
) error {
	_, exists := allowedArtifacts[name]
	if !exists {
		return fmt.Errorf("%w: %q", errUnexpectedArtifact, name)
	}

	if artifact == nil {
		return fmt.Errorf("%w: %q", errUnexpectedArtifact, name)
	}

	if !filepath.IsLocal(name) {
		return fmt.Errorf("%q: %w", name, errUnsafeArtifactName)
	}

	if artifact.Kind != ArtifactKindExecutable {
		return fmt.Errorf("%w for %q: %q", errUnsupportedArtifactKind, name, artifact.Kind)
	}

	if artifact.Size <= 0 {
		return fmt.Errorf("%w for %q", errArtifactSizeInvalid, name)
	}

	checksumErr := m.validateSHA512Base64(artifact.SHA512)
	if checksumErr != nil {
		return fmt.Errorf("artifact %q checksum: %w", name, checksumErr)
	}

	return nil
}

// validateRoleConfiguration validates role configuration metadata block.
func (m *Manifest) validateRoleConfiguration(spec *RoleSpec) error {
	cfgMeta, ok := m.Configuration[spec.Name]
	if !ok || cfgMeta == nil {
		return fmt.Errorf("configuration metadata for role %q: %w", spec.Name, errConfigArtifactMissing)
	}

	if cfgMeta.Artifact != spec.ConfigArtifact {
		return fmt.Errorf("%w %q for role %q", errUnexpectedConfigArtifact, cfgMeta.Artifact, spec.Name)
	}

	if !filepath.IsLocal(cfgMeta.Artifact) {
		return fmt.Errorf("%q: %w", cfgMeta.Artifact, errUnsafeArtifactName)
	}

	if cfgMeta.Revision == 0 {
		return errConfigRevisionInvalid
	}

	if cfgMeta.Size <= 0 {
		return errConfigSizeInvalid
	}

	if checksumErr := m.validateSHA512Base64(cfgMeta.SHA512); checksumErr != nil {
		return fmt.Errorf("configuration checksum: %w", checksumErr)
	}

	return nil
}

// NormalizeSemVer normalizes semantic version to canonical v-prefixed form.
func NormalizeSemVer(version string) (string, error) {
	return normalizeSemVer(version)
}

// compareSemVer compares semantic versions after normalization.
func (r *runner) compareSemVer(left, right string) (int, error) {
	leftVersion, err := normalizeSemVer(left)
	if err != nil {
		return 0, err
	}

	rightVersion, err := normalizeSemVer(right)
	if err != nil {
		return 0, err
	}

	return semver.Compare(leftVersion, rightVersion), nil
}

// validateSHA512Base64 validates base64 SHA-512 digest representation.
func (m *Manifest) validateSHA512Base64(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("decode base64: %w", err)
	}

	if len(decoded) != 64 {
		return fmt.Errorf("%w: expected 64 bytes, got %d", errInvalidSHA512Length, len(decoded))
	}

	return nil
}

// MarshalManifest serializes manifest bytes for signing.
func MarshalManifest(manifest *Manifest) ([]byte, error) {
	return yaml.Marshal(manifest)
}

// normalizeSemVer normalizes semantic version to canonical v-prefixed form.
func normalizeSemVer(version string) (string, error) {
	if version == "" {
		return "", errVersionIsEmpty
	}

	withPrefix := version
	if withPrefix[0] != 'v' {
		withPrefix = "v" + withPrefix
	}

	if !semver.IsValid(withPrefix) {
		return "", fmt.Errorf("%w %q", errInvalidSemanticVersion, version)
	}

	return withPrefix, nil
}

// deterministicApplyOrder returns role file order with updater executable applied last.
func (r *runner) deterministicApplyOrder() []string {
	order := make([]string, 0, len(r.spec.Files))
	updaterName := "alarm-updater" + executableExtension(runtime.GOOS)

	for _, name := range r.spec.Files {
		if name == updaterName {
			continue
		}

		order = append(order, name)
	}

	if slices.Contains(r.spec.Files, updaterName) {
		order = append(order, updaterName)
	}

	return order
}
