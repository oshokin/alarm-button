package updater

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"time"

	goupdate "github.com/doitdistributed/go-update"
	"go.yaml.in/yaml/v3"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/fsutil"
	"github.com/oshokin/alarm-button/internal/logger"
	pb "github.com/oshokin/alarm-button/internal/pb/v1"
	"github.com/oshokin/alarm-button/internal/proc"
	"github.com/oshokin/alarm-button/internal/service/common"
)

// Updater runtime constants for artifact application and checker readiness windows.
const (
	defaultExecutablePerm = fs.FileMode(0o755)
	checkerReadinessDelay = 100 * time.Millisecond
	checkerReadinessMax   = 5 * time.Second
)

// Updater execution and artifact integrity errors.
var (
	errUpdaterOptionsRequired      = errors.New("updater options are required")
	errInstallRootIsRequired       = errors.New("install root is required")
	errInsecureUpdateURLNotAllowed = errors.New(
		"insecure update URL requires localhost host or allow_insecure_update_url=true",
	)
	errBadDownloadStatus        = errors.New("bad download status")
	errArtifactSizeMismatch     = errors.New("artifact size mismatch")
	errArtifactChecksumMismatch = errors.New("artifact checksum mismatch")
	errUnexpectedProbeStatus    = errors.New("unexpected update source status")
	errUnsupportedApplyKind     = errors.New("unsupported artifact kind")
	errUnsupportedRollbackKind  = errors.New("unsupported backup kind")
	errCheckerExitedEarly       = errors.New("checker process exited before readiness")
	errCheckerPIDMismatch       = errors.New("checker pid does not match started process")
	errCheckerReadinessTimeout  = errors.New("checker readiness timed out")

	// scheduleUpdaterReplacement schedules deferred self-update replacement on Windows.
	// It is variable-backed to allow deterministic tests.
	scheduleUpdaterReplacement = scheduleWindowsUpdaterReplacement
)

// stagedArtifact describes one downloaded and verified artifact in a transaction.
type stagedArtifact struct {
	// Name is the local artifact file name inside installation root.
	Name string
	// Kind describes whether artifact is executable or configuration.
	Kind ArtifactKind
	// RemoteName is the file path served by update repository.
	RemoteName string
	// Size is expected artifact length in bytes.
	Size int64
	// SHA512 is expected base64-encoded SHA-512 checksum.
	SHA512 string
	// Revision is configuration revision for config artifacts.
	Revision uint64
	// StagedPath is absolute path to downloaded artifact in staging directory.
	StagedPath string
}

// updatePlan is the resolved set of version/config/binary changes to apply.
type updatePlan struct {
	// DesiredVersion is the version persisted after successful update.
	DesiredVersion string
	// ConfigRevision is target config revision from manifest.
	ConfigRevision uint64
	// ConfigSHA512 is target config checksum from manifest.
	ConfigSHA512 string

	// Binaries are executable artifacts that require replacement.
	Binaries []*stagedArtifact
	// Config is optional config artifact replacement.
	Config *stagedArtifact

	// NextConfig is parsed staged config used for post-restart verification.
	NextConfig *config.Config
}

// deferredUpdaterReplacement carries updater self-replacement parameters.
type deferredUpdaterReplacement struct {
	// ArtifactName is manifest file name for updater executable.
	ArtifactName string
	// StagedPath is absolute path to staged updater executable.
	StagedPath string
	// TargetPath is absolute path to installed updater executable.
	TargetPath string
}

// hasChanges reports whether plan has binary or config modifications.
func (p *updatePlan) hasChanges() bool {
	return len(p.Binaries) > 0 || p.Config != nil
}

// runner encapsulates a single updater execution lifecycle.
type runner struct {
	// role identifies updater role (client/server).
	role Role
	// spec defines role-specific executable/config contracts.
	spec *RoleSpec

	// installRoot is absolute directory with managed executables and state.
	installRoot string
	// configPath is absolute path to managed configuration file.
	configPath string
	// statePath is absolute path to updater local state file.
	statePath string

	// cfg is currently active runtime configuration.
	cfg *config.Config
	// manifest is currently fetched and verified desired state.
	manifest *Manifest
	// state is currently loaded local updater state.
	state *UpdateState

	// httpClient is used for repository downloads/probes.
	httpClient *http.Client

	// allowDowngrade allows binary version downgrade relative to local version.
	allowDowngrade bool
	// allowConfigRollback allows config revision rollback.
	allowConfigRollback bool

	// trustedManifestKeys are accepted public keys for manifest verification.
	trustedManifestKeys map[string]ed25519.PublicKey
}

// roleExecutablePath returns role executable absolute path.
func (r *runner) roleExecutablePath() string {
	return filepath.Join(r.installRoot, r.spec.Executable)
}

// rolePIDFilePath returns role PID file absolute path.
func (r *runner) rolePIDFilePath() string {
	return filepath.Join(r.installRoot, r.spec.PIDFile)
}

// Run executes updater lifecycle.
func Run(ctx context.Context, opts *Options) error {
	ctx = logger.WithName(ctx, "alarm-updater")

	if opts == nil {
		return errUpdaterOptionsRequired
	}

	role, err := parseRole(opts.UpdateType)
	if err != nil {
		return err
	}

	installRoot, err := resolveInstallRoot(opts.InstallRoot)
	if err != nil {
		return err
	}

	lockPath := filepath.Join(installRoot, UpdateLockFilename)

	lock, err := acquireUpdateLock(lockPath)
	if err != nil {
		return err
	}

	defer func() {
		if releaseErr := lock.Release(); releaseErr != nil {
			logger.ErrorKV(ctx, "Failed to release update lock", "error", releaseErr)
		}
	}()

	updaterRunner, err := newRunner(role, opts, installRoot)
	if err != nil {
		return err
	}

	return updaterRunner.run(ctx)
}

// newRunner builds an updater runner with resolved paths and loaded state.
func newRunner(role Role, opts *Options, installRoot string) (*runner, error) {
	spec, err := localRoleSpec(role)
	if err != nil {
		return nil, err
	}

	configPath, err := resolveConfigPath(opts.ConfigPath, installRoot)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	statePath := filepath.Join(installRoot, UpdateStateFilename)

	state, err := loadUpdateState(statePath)
	if err != nil {
		return nil, err
	}

	clientTimeout := cfg.Timeout
	if clientTimeout <= 0 {
		clientTimeout = config.DefaultTimeout
	}

	return &runner{
		role:        role,
		spec:        spec,
		installRoot: installRoot,
		configPath:  configPath,
		statePath:   statePath,
		cfg:         cfg,
		state:       state,
		httpClient: &http.Client{
			Timeout: clientTimeout,
		},
		allowDowngrade:      opts.AllowDowngrade,
		allowConfigRollback: opts.AllowConfigRollback,
		trustedManifestKeys: trustedManifestKeysForOptions(opts),
	}, nil
}

// resolveInstallRoot resolves installation root from override or updater executable path.
func resolveInstallRoot(override string) (string, error) {
	if override != "" {
		if !filepath.IsAbs(override) {
			abs, err := filepath.Abs(override)
			if err != nil {
				return "", fmt.Errorf("resolve install root: %w", err)
			}

			return filepath.Clean(abs), nil
		}

		return filepath.Clean(override), nil
	}

	executablePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve updater executable path: %w", err)
	}

	resolvedExecutablePath, err := filepath.EvalSymlinks(executablePath)
	if err != nil {
		resolvedExecutablePath = executablePath
	}

	installRoot := filepath.Dir(filepath.Clean(resolvedExecutablePath))
	if installRoot == "" {
		return "", errInstallRootIsRequired
	}

	return installRoot, nil
}

// resolveConfigPath resolves effective config path for updater run.
func resolveConfigPath(rawPath, installRoot string) (string, error) {
	if installRoot == "" {
		return "", errInstallRootIsRequired
	}

	if rawPath == "" || rawPath == config.DefaultConfigFilename {
		return filepath.Join(installRoot, config.DefaultConfigFilename), nil
	}

	if filepath.IsAbs(rawPath) {
		return filepath.Clean(rawPath), nil
	}

	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}

	return filepath.Clean(abs), nil
}

// trustedManifestKeysForOptions returns trusted keys with optional runtime override.
func trustedManifestKeysForOptions(opts *Options) map[string]ed25519.PublicKey {
	if len(opts.TrustedManifestKeys) == 0 {
		return trustedUpdatePublicKeys
	}

	cloned := make(map[string]ed25519.PublicKey, len(opts.TrustedManifestKeys))
	for keyID, key := range opts.TrustedManifestKeys {
		cloned[keyID] = append(ed25519.PublicKey(nil), key...)
	}

	return cloned
}

// run executes full updater flow: fetch, verify, plan, apply, and persist state.
//
//nolint:cyclop,funlen // Update runner intentionally keeps workflow order explicit.
func (r *runner) run(ctx context.Context) error {
	manifestBytes, signatureBytes, err := r.fetchManifestAndSignature(ctx)
	if err != nil {
		return err
	}

	err = r.verifyManifest(manifestBytes, signatureBytes)
	if err != nil {
		return err
	}

	manifest, err := r.parseManifest(manifestBytes)
	if err != nil {
		return err
	}

	err = manifest.Validate(r.spec)
	if err != nil {
		return err
	}

	r.manifest = manifest

	plan, err := r.planUpdate(ctx)
	if err != nil {
		return err
	}

	if !plan.hasChanges() {
		logger.Info(ctx, "No update required")
		return r.persistUpdateState(plan)
	}

	transactionDir, err := os.MkdirTemp(r.installRoot, ".update-transaction-*")
	if err != nil {
		return fmt.Errorf("create transaction directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(transactionDir) }()

	stagingDir := filepath.Join(transactionDir, "staged")
	backupDir := filepath.Join(transactionDir, "backup")

	err = os.MkdirAll(stagingDir, 0o700)
	if err != nil {
		return fmt.Errorf("create staging directory: %w", err)
	}

	err = os.MkdirAll(backupDir, 0o700)
	if err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	err = r.downloadArtifacts(ctx, stagingDir, plan)
	if err != nil {
		return err
	}

	err = r.preflightConfig(ctx, plan)
	if err != nil {
		return err
	}

	err = r.applyTransaction(ctx, backupDir, plan)
	if err != nil {
		return err
	}

	return r.persistUpdateState(plan)
}

// parseManifest decodes manifest YAML with strict field validation.
func (r *runner) parseManifest(data []byte) (*Manifest, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	return &manifest, nil
}

// fetchManifestAndSignature downloads signed manifest payload and detached signature.
func (r *runner) fetchManifestAndSignature(ctx context.Context) ([]byte, []byte, error) {
	manifestBytes, err := r.downloadBoundedFile(ctx, VersionFilename, maxManifestSize)
	if err != nil {
		return nil, nil, fmt.Errorf("download manifest: %w", err)
	}

	signatureBytes, err := r.downloadBoundedFile(ctx, SignatureFilename, 4*1024)
	if err != nil {
		return nil, nil, fmt.Errorf("download signature: %w", err)
	}

	return manifestBytes, signatureBytes, nil
}

// downloadBoundedFile downloads repository file with explicit size limit.
func (r *runner) downloadBoundedFile(ctx context.Context, name string, maxBytes int64) ([]byte, error) {
	resp, err := r.getFileBodyFromServer(ctx, name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	reader := io.LimitReader(resp.Body, maxBytes+1)

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if int64(len(data)) > maxBytes {
		return nil, errManifestTooLarge
	}

	return data, nil
}

// getFileBodyFromServer sends repository download request for one artifact file.
func (r *runner) getFileBodyFromServer(ctx context.Context, fileName string) (*http.Response, error) {
	baseURL, err := url.Parse(r.cfg.ServerUpdateFolder)
	if err != nil {
		return nil, fmt.Errorf("parse update URL: %w", err)
	}

	if baseURL.Scheme == "http" && !r.cfg.AllowInsecureUpdateURL && !r.isLoopbackHost(baseURL.Hostname()) {
		return nil, errInsecureUpdateURLNotAllowed
	}

	baseURL.Path = path.Join(baseURL.Path, fileName)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	response, err := r.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", fileName, err)
	}

	if response.StatusCode != http.StatusOK {
		defer func() { _ = response.Body.Close() }()
		return nil, fmt.Errorf("%w for %s: %s", errBadDownloadStatus, fileName, response.Status)
	}

	return response, nil
}

// planUpdate resolves desired binaries/config and policy checks against local state.
//
//nolint:cyclop // Update planning combines version, checksum, and rollback policy checks.
func (r *runner) planUpdate(ctx context.Context) (*updatePlan, error) {
	cfgMeta := r.manifest.Configuration[r.role]
	if cfgMeta == nil {
		return nil, fmt.Errorf("configuration metadata for role %q: %w", r.role, errConfigArtifactMissing)
	}

	plan := &updatePlan{
		ConfigRevision: cfgMeta.Revision,
		ConfigSHA512:   cfgMeta.SHA512,
	}

	localVersion, err := r.detectLocalVersion(ctx)
	if err != nil {
		return nil, err
	}

	manifestHasRoleBinaries, err := r.collectBinaryUpdates(plan)
	if err != nil {
		return nil, err
	}

	err = r.validateBinaryVersionPolicy(localVersion, manifestHasRoleBinaries)
	if err != nil {
		return nil, err
	}

	plan.DesiredVersion = r.resolveDesiredVersion(localVersion, manifestHasRoleBinaries)

	if r.state.ConfigRevision == cfgMeta.Revision && r.state.ConfigSHA512 != "" &&
		r.state.ConfigSHA512 != cfgMeta.SHA512 {
		return nil, errConfigRevisionReused
	}

	if cfgMeta.Revision < r.state.ConfigRevision && !r.allowConfigRollback {
		return nil, errConfigRollbackRejected
	}

	localConfigChecksum, err := r.checksumPathBase64(r.configPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("checksum local config: %w", err)
	}

	if localConfigChecksum != cfgMeta.SHA512 || r.state.ConfigRevision != cfgMeta.Revision {
		plan.Config = &stagedArtifact{
			Name:       config.DefaultConfigFilename,
			Kind:       ArtifactKindConfig,
			RemoteName: cfgMeta.Artifact,
			Size:       cfgMeta.Size,
			SHA512:     cfgMeta.SHA512,
			Revision:   cfgMeta.Revision,
		}
	}

	return plan, nil
}

// collectBinaryUpdates compares local role binaries with manifest checksums.
func (r *runner) collectBinaryUpdates(plan *updatePlan) (bool, error) {
	manifestHasRoleBinaries := false

	for _, fileName := range r.spec.Files {
		artifact, exists := r.manifest.Artifacts[fileName]
		if !exists {
			continue
		}

		if artifact == nil {
			return false, fmt.Errorf("%w: %q", errUnexpectedArtifact, fileName)
		}

		manifestHasRoleBinaries = true
		localArtifactPath := filepath.Join(r.installRoot, fileName)

		localChecksum, checksumErr := r.checksumPathBase64(localArtifactPath)
		if checksumErr != nil && !errors.Is(checksumErr, os.ErrNotExist) {
			return false, fmt.Errorf("checksum local artifact %q: %w", localArtifactPath, checksumErr)
		}

		if localChecksum == artifact.SHA512 {
			continue
		}

		plan.Binaries = append(plan.Binaries, &stagedArtifact{
			Name:       fileName,
			Kind:       ArtifactKindExecutable,
			RemoteName: fileName,
			Size:       artifact.Size,
			SHA512:     artifact.SHA512,
		})
	}

	return manifestHasRoleBinaries, nil
}

// validateBinaryVersionPolicy enforces downgrade policy for binary-changing updates.
func (r *runner) validateBinaryVersionPolicy(
	localVersion string,
	manifestHasRoleBinaries bool,
) error {
	if !manifestHasRoleBinaries || localVersion == "" {
		return nil
	}

	comparison, compareErr := r.compareSemVer(r.manifest.Version, localVersion)
	if compareErr != nil {
		return fmt.Errorf("compare local and remote versions: %w", compareErr)
	}

	if comparison < 0 && !r.allowDowngrade {
		return errBinaryDowngradeRejected
	}

	return nil
}

// resolveDesiredVersion computes persisted app version for binary or config-only updates.
func (r *runner) resolveDesiredVersion(
	localVersion string,
	manifestHasRoleBinaries bool,
) string {
	if manifestHasRoleBinaries {
		return r.manifest.Version
	}

	if localVersion != "" {
		return localVersion
	}

	if r.state.ApplicationVersion != "" {
		return r.state.ApplicationVersion
	}

	return r.manifest.Version
}

// checksumPathBase64 returns base64-encoded SHA-512 of file contents.
func (r *runner) checksumPathBase64(path string) (string, error) {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hasher := sha512.New()
	if _, err = io.Copy(hasher, file); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(hasher.Sum(nil)), nil
}

// downloadArtifacts downloads all planned binary/config artifacts into staging directory.
func (r *runner) downloadArtifacts(ctx context.Context, stagingDir string, plan *updatePlan) error {
	root, err := os.OpenRoot(stagingDir)
	if err != nil {
		return fmt.Errorf("open staging root: %w", err)
	}
	defer func() { _ = root.Close() }()

	for _, artifact := range plan.Binaries {
		if artifact == nil {
			continue
		}

		err = r.downloadArtifact(ctx, root, artifact)
		if err != nil {
			return err
		}
	}

	if plan.Config != nil {
		err = r.downloadArtifact(ctx, root, plan.Config)
		if err != nil {
			return err
		}
	}

	return nil
}

// downloadArtifact downloads one artifact and verifies size/checksum before staging.
//
//nolint:cyclop // Download path includes explicit integrity and fsync checks.
func (r *runner) downloadArtifact(ctx context.Context, root *os.Root, artifact *stagedArtifact) error {
	if !filepath.IsLocal(artifact.RemoteName) {
		return fmt.Errorf("%q: %w", artifact.RemoteName, errUnsafeArtifactName)
	}

	response, err := r.getFileBodyFromServer(ctx, artifact.RemoteName)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	file, err := root.Create(artifact.RemoteName)
	if err != nil {
		return fmt.Errorf("create staged file: %w", err)
	}

	limited := io.LimitReader(response.Body, artifact.Size+1)
	hasher := sha512.New()

	written, copyErr := io.Copy(io.MultiWriter(file, hasher), limited)
	if syncErr := file.Sync(); syncErr != nil && copyErr == nil {
		copyErr = syncErr
	}

	if closeErr := file.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}

	if copyErr != nil {
		return fmt.Errorf("write staged file: %w", copyErr)
	}

	if written != artifact.Size {
		return fmt.Errorf(
			"%w for %s: got %d, want %d",
			errArtifactSizeMismatch,
			artifact.RemoteName,
			written,
			artifact.Size,
		)
	}

	actualChecksum := base64.StdEncoding.EncodeToString(hasher.Sum(nil))
	if actualChecksum != artifact.SHA512 {
		return fmt.Errorf("%w for %s", errArtifactChecksumMismatch, artifact.RemoteName)
	}

	artifact.StagedPath = filepath.Join(root.Name(), artifact.RemoteName)

	return nil
}

// preflightConfig validates staged config and probes update source transition safety.
func (r *runner) preflightConfig(ctx context.Context, plan *updatePlan) error {
	if plan.Config == nil {
		return nil
	}

	content, err := os.ReadFile(filepath.Clean(plan.Config.StagedPath))
	if err != nil {
		return fmt.Errorf("read staged config: %w", err)
	}

	nextCfg, err := config.Parse(content)
	if err != nil {
		return err
	}

	err = nextCfg.ValidateLocalEnvironment()
	if err != nil {
		return err
	}

	plan.NextConfig = nextCfg

	if nextCfg.ServerUpdateFolder != r.cfg.ServerUpdateFolder {
		probeErr := r.probeUpdateSource(ctx, nextCfg.ServerUpdateFolder, nextCfg.Timeout)
		if probeErr != nil {
			return fmt.Errorf("new update source preflight failed: %w", probeErr)
		}
	}

	return nil
}

// probeUpdateSource validates that next update source can serve signed manifest.
func (r *runner) probeUpdateSource(ctx context.Context, rawURL string, timeout time.Duration) error {
	if rawURL == "" {
		return nil
	}

	if timeout <= 0 {
		timeout = config.DefaultTimeout
	}

	baseURL, err := url.Parse(rawURL)
	if err != nil {
		return err
	}

	baseURL.Path = path.Join(baseURL.Path, VersionFilename)

	client := &http.Client{Timeout: timeout}

	request, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL.String(), http.NoBody)
	if err != nil {
		return err
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s", errUnexpectedProbeStatus, response.Status)
	}

	return nil
}

// backupEntry stores rollback metadata for one target file.
type backupEntry struct {
	// TargetPath is absolute install path for artifact target.
	TargetPath string
	// Exists indicates whether target existed before transaction.
	Exists bool
	// BackupPath is path to backup copy when target existed.
	BackupPath string
	// Mode is original file mode preserved for rollback.
	Mode fs.FileMode
	// Kind is artifact kind used by rollback strategy.
	Kind ArtifactKind
}

// prepareBackups snapshots targets before in-place replacement for rollback safety.
func (r *runner) prepareBackups(
	backupDir string,
	artifacts []*stagedArtifact,
) (map[string]*backupEntry, error) {
	backups := make(map[string]*backupEntry, len(artifacts))

	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}

		targetPath, err := r.targetPathForArtifact(artifact)
		if err != nil {
			return nil, err
		}

		entry := &backupEntry{
			TargetPath: targetPath,
			Kind:       artifact.Kind,
			Mode:       config.DefaultFilePermissions,
		}

		info, err := os.Stat(targetPath)
		if err == nil {
			entry.Exists = true
			entry.Mode = info.Mode()
			entry.BackupPath = filepath.Join(backupDir, filepath.Base(targetPath)+".bak")

			content, readErr := os.ReadFile(filepath.Clean(targetPath))
			if readErr != nil {
				return nil, fmt.Errorf("read backup source %q: %w", targetPath, readErr)
			}

			if writeErr := fsutil.WriteFileAtomic(entry.BackupPath, content, info.Mode()); writeErr != nil {
				return nil, fmt.Errorf("write backup %q: %w", entry.BackupPath, writeErr)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("stat target %q: %w", targetPath, err)
		}

		backups[targetPath] = entry
	}

	return backups, nil
}

// applyTransaction applies plan with rollback on any failure.
//
//nolint:cyclop,funlen,gocognit // Transaction flow is intentionally linear for rollback safety.
func (r *runner) applyTransaction(ctx context.Context, backupDir string, plan *updatePlan) (retErr error) {
	order := r.deterministicApplyOrder()

	targets, deferredUpdater, err := r.collectApplyTargets(plan, order)
	if err != nil {
		return err
	}

	backups, err := r.prepareBackups(backupDir, targets)
	if err != nil {
		return err
	}

	stopped := false

	var startedProcess *os.Process

	defer func() {
		if retErr == nil {
			return
		}

		if stopErr := r.stopStartedProcess(startedProcess); stopErr != nil {
			retErr = fmt.Errorf("%w; stop failed new process: %w", retErr, stopErr)
		}

		if rollbackErr := r.rollbackTargets(targets, backups); rollbackErr != nil {
			retErr = fmt.Errorf("%w; rollback failed: %w", retErr, rollbackErr)
			return
		}

		if stopped {
			_, startErr := r.startExecutable()
			if startErr != nil {
				retErr = fmt.Errorf("%w; restart old process failed: %w", retErr, startErr)
			}
		}
	}()

	err = r.stopProcessByPIDFile()
	if err != nil {
		return err
	}

	stopped = true

	for _, artifact := range targets {
		if artifact == nil {
			continue
		}

		targetPath, targetErr := r.targetPathForArtifact(artifact)
		if targetErr != nil {
			return targetErr
		}

		switch artifact.Kind {
		case ArtifactKindExecutable:
			err = r.applyExecutable(
				artifact.StagedPath,
				targetPath,
				artifact.SHA512,
				defaultExecutablePerm,
			)
			if err != nil {
				return fmt.Errorf("apply executable %q: %w", artifact.Name, err)
			}
		case ArtifactKindConfig:
			content, readErr := os.ReadFile(filepath.Clean(artifact.StagedPath))
			if readErr != nil {
				return fmt.Errorf("read staged config: %w", readErr)
			}

			writeErr := fsutil.WriteFileAtomic(targetPath, content, config.DefaultFilePermissions)
			if writeErr != nil {
				return fmt.Errorf("replace config: %w", writeErr)
			}
		default:
			return fmt.Errorf("%w %q", errUnsupportedApplyKind, artifact.Kind)
		}
	}

	startedProcess, err = r.startExecutable()
	if err != nil {
		return fmt.Errorf("start role executable: %w", err)
	}

	err = r.verifyStart(ctx, plan, startedProcess)
	if err != nil {
		return err
	}

	if deferredUpdater != nil {
		err = scheduleUpdaterReplacement(os.Getpid(), deferredUpdater.StagedPath, deferredUpdater.TargetPath)
		if err != nil {
			return fmt.Errorf("schedule deferred updater replacement for %q: %w", deferredUpdater.ArtifactName, err)
		}
	}

	return nil
}

// collectApplyTargets orders apply targets and isolates deferred updater replacement.
func (r *runner) collectApplyTargets(
	plan *updatePlan,
	order []string,
) ([]*stagedArtifact, *deferredUpdaterReplacement, error) {
	targets := make([]*stagedArtifact, 0, len(plan.Binaries)+1)
	byName := make(map[string]*stagedArtifact, len(plan.Binaries))

	for _, item := range plan.Binaries {
		if item == nil {
			continue
		}

		byName[item.Name] = item
	}

	var deferredUpdater *deferredUpdaterReplacement

	for _, name := range order {
		artifact, exists := byName[name]
		if !exists {
			continue
		}

		targetPath, err := r.targetPathForArtifact(artifact)
		if err != nil {
			return nil, nil, err
		}

		if r.shouldDeferUpdaterReplacement(artifact.Name) {
			deferredUpdater = &deferredUpdaterReplacement{
				ArtifactName: artifact.Name,
				StagedPath:   artifact.StagedPath,
				TargetPath:   targetPath,
			}

			continue
		}

		targets = append(targets, artifact)
	}

	if plan.Config != nil {
		targets = append(targets, plan.Config)
	}

	return targets, deferredUpdater, nil
}

// shouldDeferUpdaterReplacement reports whether artifact is Windows updater executable.
func (r *runner) shouldDeferUpdaterReplacement(artifactName string) bool {
	if filepath.Ext(r.spec.Executable) != executableExtension(goosWindows) {
		return false
	}

	updaterExecutableName := "alarm-updater" + executableExtension(goosWindows)

	return artifactName == updaterExecutableName
}

// rollbackTargets restores original targets in reverse order.
//
//nolint:cyclop,gocognit // Rollback explicitly mirrors forward apply paths for reliability.
func (r *runner) rollbackTargets(
	targets []*stagedArtifact,
	backups map[string]*backupEntry,
) error {
	for _, artifact := range slices.Backward(targets) {
		if artifact == nil {
			continue
		}

		targetPath, err := r.targetPathForArtifact(artifact)
		if err != nil {
			return err
		}

		backup := backups[targetPath]
		if backup == nil || !backup.Exists {
			removeErr := os.Remove(filepath.Clean(targetPath))
			if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				return fmt.Errorf("remove new target %q: %w", targetPath, removeErr)
			}

			continue
		}

		switch backup.Kind {
		case ArtifactKindExecutable:
			checksum, checksumErr := r.checksumPathBase64(backup.BackupPath)
			if checksumErr != nil {
				return fmt.Errorf("checksum backup executable %q: %w", backup.BackupPath, checksumErr)
			}

			applyErr := r.applyExecutable(backup.BackupPath, targetPath, checksum, defaultExecutablePerm)
			if applyErr != nil {
				return fmt.Errorf("rollback executable %q: %w", targetPath, applyErr)
			}
		case ArtifactKindConfig:
			content, readErr := os.ReadFile(filepath.Clean(backup.BackupPath))
			if readErr != nil {
				return fmt.Errorf("read config backup %q: %w", backup.BackupPath, readErr)
			}

			writeErr := fsutil.WriteFileAtomic(targetPath, content, config.DefaultFilePermissions)
			if writeErr != nil {
				return fmt.Errorf("rollback config %q: %w", targetPath, writeErr)
			}
		default:
			return fmt.Errorf("%w %q", errUnsupportedRollbackKind, backup.Kind)
		}
	}

	return nil
}

// applyExecutable applies executable replacement with checksum validation.
func (r *runner) applyExecutable(stagedPath, targetPath, checksumB64 string, mode fs.FileMode) error {
	checksum, err := base64.StdEncoding.DecodeString(checksumB64)
	if err != nil {
		return err
	}

	source, err := os.Open(filepath.Clean(stagedPath))
	if err != nil {
		return err
	}
	defer func() { _ = source.Close() }()

	options := goupdate.Options{
		TargetPath: targetPath,
		TargetMode: mode,
		Checksum:   checksum,
		Hash:       crypto.SHA512,
	}

	return goupdate.Apply(source, options)
}

// verifyStart runs role-specific post-start readiness checks.
func (r *runner) verifyStart(ctx context.Context, plan *updatePlan, startedProcess *os.Process) error {
	switch r.role {
	case RoleServer:
		return r.verifyServerStart(ctx, plan)
	case RoleClient:
		return r.verifyCheckerStart(ctx, startedProcess)
	default:
		return nil
	}
}

// verifyServerStart checks restarted server health via local gRPC endpoint.
func (r *runner) verifyServerStart(ctx context.Context, plan *updatePlan) error {
	settings := r.cfg
	if plan.NextConfig != nil {
		settings = plan.NextConfig
	}

	listenAddress := settings.ListenAddress
	if listenAddress == "" {
		listenAddress = config.DefaultListenAddress
	}

	listenAddress = r.normalizeListenAddressForDial(listenAddress)

	client, err := common.NewClient(
		listenAddress,
		settings,
		common.WithCallTimeout(settings.Timeout),
	)
	if err != nil {
		return fmt.Errorf("verify server restart dial: %w", err)
	}
	defer func() { _ = client.Close() }()

	checkCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()

	err = client.CheckHealth(checkCtx, pb.AlarmService_ServiceDesc.ServiceName)
	if err != nil {
		return fmt.Errorf("verify server restart health: %w", err)
	}

	return nil
}

// verifyCheckerStart checks checker liveness and PID ownership after restart.
func (r *runner) verifyCheckerStart(ctx context.Context, startedProcess *os.Process) error {
	if startedProcess == nil {
		return errCheckerExitedEarly
	}

	checkCtx, cancel := context.WithTimeout(ctx, checkerReadinessMax)
	defer cancel()

	for {
		ready, err := r.checkCheckerReadiness(startedProcess.Pid)
		if err != nil {
			return err
		}

		if ready {
			return nil
		}

		select {
		case <-checkCtx.Done():
			return fmt.Errorf("%w: %s", errCheckerReadinessTimeout, checkerReadinessMax)
		case <-time.After(checkerReadinessDelay):
		}
	}
}

// checkCheckerReadiness validates checker PID file ownership and liveness.
func (r *runner) checkCheckerReadiness(expectedPID int) (bool, error) {
	pidPath := r.rolePIDFilePath()

	pidFromFile, err := proc.Read(pidPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, fmt.Errorf("read checker pid file: %w", err)
		}

		running, runningErr := isProcessRunning(expectedPID)
		if runningErr != nil {
			return false, runningErr
		}

		if !running {
			return false, fmt.Errorf("%w: pid %d", errCheckerExitedEarly, expectedPID)
		}

		return false, nil
	}

	if pidFromFile != expectedPID {
		return false, fmt.Errorf("%w: got %d, want %d", errCheckerPIDMismatch, pidFromFile, expectedPID)
	}

	running, err := isProcessRunning(expectedPID)
	if err != nil {
		return false, err
	}

	if !running {
		return false, fmt.Errorf("%w: pid %d", errCheckerExitedEarly, expectedPID)
	}

	return true, nil
}

// persistUpdateState writes applied desired-state metadata for next runs.
func (r *runner) persistUpdateState(plan *updatePlan) error {
	next := &UpdateState{
		Role:               r.role,
		ApplicationVersion: plan.DesiredVersion,
		ConfigRevision:     plan.ConfigRevision,
		ConfigSHA512:       plan.ConfigSHA512,
	}

	if err := r.saveUpdateState(next); err != nil {
		return err
	}

	r.state = next
	if plan.NextConfig != nil {
		r.cfg = plan.NextConfig
	}

	return nil
}

// isLoopbackHost reports whether host is localhost or loopback IP.
func (r *runner) isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

// normalizeListenAddressForDial replaces wildcard hosts with local loopback addresses.
func (r *runner) normalizeListenAddressForDial(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}

	switch host {
	case "", "0.0.0.0":
		return net.JoinHostPort("127.0.0.1", port)
	case "::":
		return net.JoinHostPort("::1", port)
	default:
		return address
	}
}
