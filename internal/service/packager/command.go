package packager

import (
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/fsutil"
	"github.com/oshokin/alarm-button/internal/service/updater"
)

// Manifest and signature file permission defaults.
const (
	defaultManifestPermission = 0o644
)

// Packager input validation and artifact processing errors.
var (
	errPackagerOptionsRequired      = errors.New("packager options are required")
	errOutputDirectoryRequired      = errors.New("output directory is required")
	errPrivateKeyPathRequired       = errors.New("private key path is required")
	errVersionRequired              = errors.New("version is required")
	errInvalidReleaseVersion        = errors.New("invalid release version")
	errPrivateKeyPEMBlockNotFound   = errors.New("private key PEM block not found")
	errPrivateKeyNotEd25519         = errors.New("private key is not Ed25519")
	errConfigRevisionMustBePositive = errors.New("configuration revision must be positive")
	errSourcePathIsNotARegularFile  = errors.New("source path is not a regular file")
	errPackageContentRequired       = errors.New("either input directory or role configuration is required")
	errClientConfigRequired         = errors.New("client config is required when binaries are published")
	errServerConfigRequired         = errors.New("server config is required when binaries are published")
	errKeyIDRequired                = errors.New("manifest signing key id is required")
)

// Options contains inputs for package generation.
type Options struct {
	// InputDir is directory with built executables for selected platform.
	InputDir string
	// OutputDir is destination directory for artifacts and metadata.
	OutputDir string
	// Version is release semantic version.
	Version string
	// SigningKey is path to Ed25519 private key in PKCS8 PEM.
	SigningKey string

	// GOOS is target operating system for artifact map selection.
	GOOS string
	// GOARCH is target architecture for manifest platform fields.
	GOARCH string

	// ClientConfig is optional client configuration YAML path.
	ClientConfig string
	// ClientConfigRevision is client config monotonic revision.
	ClientConfigRevision uint64
	// ServerConfig is optional server configuration YAML path.
	ServerConfig string
	// ServerConfigRevision is server config monotonic revision.
	ServerConfigRevision uint64

	// KeyID identifies trusted signing key referenced by manifest metadata.
	KeyID string
}

// Run builds signed update metadata and copies artifacts into output directory.
func Run(opts *Options) error {
	runOpts, privateKey, err := prepareRunInputs(opts)
	if err != nil {
		return err
	}

	err = os.MkdirAll(runOpts.OutputDir, 0o750)
	if err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	manifest := &updater.Manifest{
		SchemaVersion: updater.CurrentManifestSchema,
		Version:       runOpts.Version,
		Platform: updater.Platform{
			GOOS:   runOpts.GOOS,
			GOARCH: runOpts.GOARCH,
		},
		Signing: updater.Signing{
			KeyID: runOpts.KeyID,
		},
		Artifacts:     make(map[string]*updater.Artifact),
		Configuration: make(map[updater.Role]*updater.ConfigArtifact),
	}

	if runOpts.InputDir != "" {
		err = collectExecutables(runOpts.InputDir, runOpts.OutputDir, runOpts.GOOS, manifest)
		if err != nil {
			return err
		}
	}

	err = collectRoleConfig(
		updater.RoleClient,
		runOpts.ClientConfig,
		runOpts.ClientConfigRevision,
		runOpts.OutputDir,
		manifest,
	)
	if err != nil {
		return err
	}

	err = collectRoleConfig(
		updater.RoleServer,
		runOpts.ServerConfig,
		runOpts.ServerConfigRevision,
		runOpts.OutputDir,
		manifest,
	)
	if err != nil {
		return err
	}

	manifestBytes, err := updater.MarshalManifest(manifest)
	if err != nil {
		return err
	}

	signature := ed25519.Sign(privateKey, manifestBytes)
	signatureText := base64.StdEncoding.EncodeToString(signature) + "\n"

	err = fsutil.WriteFileAtomic(
		filepath.Join(runOpts.OutputDir, updater.VersionFilename),
		manifestBytes,
		defaultManifestPermission,
	)
	if err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	err = fsutil.WriteFileAtomic(
		filepath.Join(runOpts.OutputDir, updater.SignatureFilename),
		[]byte(signatureText),
		defaultManifestPermission,
	)
	if err != nil {
		return fmt.Errorf("write manifest signature: %w", err)
	}

	return nil
}

// prepareRunInputs validates options, normalizes values, and loads signing key.
func prepareRunInputs(opts *Options) (*Options, ed25519.PrivateKey, error) {
	if err := validateRunInputs(opts); err != nil {
		return nil, nil, err
	}

	copied := *opts

	version, err := normalizeVersion(copied.Version)
	if err != nil {
		return nil, nil, err
	}

	copied.Version = version

	applyPlatformDefaults(&copied)

	privateKey, err := loadPrivateKey(copied.SigningKey)
	if err != nil {
		return nil, nil, err
	}

	return &copied, privateKey, nil
}

// validateRunInputs validates required fields and content contract.
func validateRunInputs(opts *Options) error {
	if err := validateRequiredRunFields(opts); err != nil {
		return err
	}

	return validateRunContentOptions(opts)
}

// validateRequiredRunFields validates mandatory top-level CLI options.
func validateRequiredRunFields(opts *Options) error {
	if opts == nil {
		return errPackagerOptionsRequired
	}

	if opts.OutputDir == "" {
		return errOutputDirectoryRequired
	}

	if opts.SigningKey == "" {
		return errPrivateKeyPathRequired
	}

	if opts.KeyID == "" {
		return errKeyIDRequired
	}

	return nil
}

// validateRunContentOptions validates package content constraints by mode.
func validateRunContentOptions(opts *Options) error {
	if opts.InputDir == "" && opts.ClientConfig == "" && opts.ServerConfig == "" {
		return errPackageContentRequired
	}

	if opts.InputDir == "" {
		return nil
	}

	if opts.ClientConfig == "" {
		return errClientConfigRequired
	}

	if opts.ServerConfig == "" {
		return errServerConfigRequired
	}

	return nil
}

// applyPlatformDefaults applies GOOS/GOARCH defaults from current runtime.
func applyPlatformDefaults(opts *Options) {
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}

	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
}

// normalizeVersion validates and canonicalizes release semantic version.
func normalizeVersion(version string) (string, error) {
	if version == "" {
		return "", errVersionRequired
	}

	normalized, err := updater.NormalizeSemVer(version)
	if err != nil {
		return "", err
	}

	switch normalized {
	case "vdev", "vunknown", "vnone":
		return "", fmt.Errorf("%w %q", errInvalidReleaseVersion, version)
	}

	return normalized, nil
}

// loadPrivateKey loads Ed25519 PKCS8 private key from PEM file.
func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	pemBytes, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errPrivateKeyPEMBlockNotFound
	}

	keyAny, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS8 private key: %w", err)
	}

	key, ok := keyAny.(ed25519.PrivateKey)
	if !ok {
		return nil, errPrivateKeyNotEd25519
	}

	return key, nil
}

// collectExecutables copies executable artifacts and records manifest metadata.
func collectExecutables(inputDir, outputDir, goos string, manifest *updater.Manifest) error {
	specs := updater.RoleSpecsForPlatform(goos)

	uniqueNames := map[string]struct{}{}

	for _, spec := range specs {
		for _, fileName := range spec.Files {
			uniqueNames[fileName] = struct{}{}
		}
	}

	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		sourcePath := filepath.Join(inputDir, name)
		targetPath := filepath.Join(outputDir, name)

		size, checksum, err := copyRegularFile(sourcePath, targetPath, defaultManifestPermission)
		if err != nil {
			return err
		}

		manifest.Artifacts[name] = &updater.Artifact{
			Kind:   updater.ArtifactKindExecutable,
			Size:   size,
			SHA512: checksum,
		}
	}

	return nil
}

// collectRoleConfig validates and stages role configuration artifact metadata.
func collectRoleConfig(
	role updater.Role,
	path string,
	revision uint64,
	outputDir string,
	manifest *updater.Manifest,
) error {
	if path == "" {
		return nil
	}

	if revision == 0 {
		return fmt.Errorf("%w for role %q", errConfigRevisionMustBePositive, role)
	}

	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("read %s config: %w", role, err)
	}

	_, err = config.Parse(content)
	if err != nil {
		return fmt.Errorf("validate %s config: %w", role, err)
	}

	artifactName := updater.ConfigArtifactName(role)
	outputPath := filepath.Join(outputDir, artifactName)

	size, checksum, err := copyRegularFile(path, outputPath, config.DefaultFilePermissions)
	if err != nil {
		return err
	}

	manifest.Configuration[role] = &updater.ConfigArtifact{
		Revision: revision,
		Artifact: artifactName,
		Size:     size,
		SHA512:   checksum,
	}

	return nil
}

// copyRegularFile copies regular file and returns written size and SHA-512 checksum.
func copyRegularFile(source, destination string, mode os.FileMode) (int64, string, error) {
	info, err := os.Lstat(filepath.Clean(source))
	if err != nil {
		return 0, "", fmt.Errorf("stat %q: %w", source, err)
	}

	if !info.Mode().IsRegular() {
		return 0, "", fmt.Errorf("%w: %s", errSourcePathIsNotARegularFile, source)
	}

	input, err := os.Open(filepath.Clean(source))
	if err != nil {
		return 0, "", fmt.Errorf("open %q: %w", source, err)
	}
	defer func() { _ = input.Close() }()

	output, err := os.OpenFile(filepath.Clean(destination), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return 0, "", fmt.Errorf("open output %q: %w", destination, err)
	}

	hasher := sha512.New()

	written, copyErr := io.Copy(io.MultiWriter(output, hasher), input)
	if syncErr := output.Sync(); syncErr != nil && copyErr == nil {
		copyErr = syncErr
	}

	if closeErr := output.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}

	if copyErr != nil {
		return 0, "", fmt.Errorf("copy %q: %w", source, copyErr)
	}

	return written, base64.StdEncoding.EncodeToString(hasher.Sum(nil)), nil
}
