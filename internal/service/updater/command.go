package updater

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	goupdate "github.com/doitdistributed/go-update"
	"github.com/mitchellh/go-ps"
	"gopkg.in/yaml.v3"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	"github.com/oshokin/alarm-button/internal/service/common"
)

var (
	errUpdaterAlreadyRunning  = errors.New("the updater is already running")
	errSettingsNotInitialised = errors.New("settings are not initialized")
	errEmptyDescription       = errors.New("update description is empty")
	errNoRoleFiles            = errors.New("unable to find files for role")
	errNoChecksum             = errors.New("checksum missing for file")
	errBadHTTPStatus          = errors.New("unexpected http status")
	errNoRoleExecutable       = errors.New("unable to find executable for role")
	errUnsupportedOS          = errors.New("os not supported")
	errInvalidVersionOutput   = errors.New("invalid version output format")
	errUnknownUpdateType      = errors.New("unknown update type")
)

// Options are inputs accepted by the updater entry point.
type Options struct {
	// ConfigPath is the optional path to settings YAML file.
	ConfigPath string
	// UpdateType is the role to update for (client or server).
	UpdateType string
}

// runner holds the mutable state and helpers for a single update execution.
// It is intentionally unexported—call Run(ctx, Options) from callers.
type runner struct {
	description        *Description      // Remote manifest describing the release.
	cfg                *config.Config    // Connection configuration loaded from YAML.
	localVersion       string            // Detected local version.
	IsUpdateNeeded     bool              // Whether client files differ from server checksums.
	temporaryDirectory string            // Where new files are downloaded before apply.
	downloadedFiles    map[string]string // Logical name -> local temp path.
}

// Run executes the updater lifecycle and is the public entry point for the CLI.
func Run(ctx context.Context, opts *Options) error {
	// Set context with logger name for tracking.
	ctx = logger.WithName(ctx, "alarm-updater")

	up, err := newRunner(ctx, opts)
	if err != nil {
		return err
	}

	defer up.cleanup(ctx)

	if err = up.Run(ctx); err != nil {
		logger.ErrorKV(ctx, "Updater run failed", "error", err)
		return err
	}

	logger.Info(ctx, "Updater completed")

	return nil
}

// newRunner prepares the run and writes a marker to avoid concurrent runs.
// It also ensures we can reach the server before doing any work.
func newRunner(ctx context.Context, opts *Options) (*runner, error) {
	u := &runner{
		downloadedFiles: make(map[string]string, defaultMapCapacity),
	}

	if IsUpdaterRunningNow(ctx) {
		return u, errUpdaterAlreadyRunning
	}

	updateMarker, err := os.Create(MarkerFilename)
	if err != nil {
		return u, err
	}

	if err = updateMarker.Close(); err != nil {
		return u, err
	}

	configPath := opts.ConfigPath
	if configPath == "" {
		configPath = config.DefaultConfigFilename
	}

	var settings *config.Config

	settings, err = config.Load(configPath)
	if err != nil {
		return u, err
	}

	settings.UpdateType = strings.TrimSpace(opts.UpdateType)
	u.cfg = settings

	if err = u.ensureServerReachable(ctx); err != nil {
		return u, err
	}

	return u, nil
}

// Run executes the enhanced workflow for this runner instance:
// 1) Stop known processes.
// 2) Detect local version.
// 3) Fetch remote manifest.
// 4) Compare versions.
// 5) Verify checksums.
// 6) Download and apply files if needed.
// 7) Start the target executable.
func (u *runner) Run(ctx context.Context) error {
	// Preparation.
	if err := u.prepareForUpdate(ctx); err != nil {
		return err
	}

	// Determine if update is needed.
	versionUpdateNeeded, err := u.determineUpdateNeeded(ctx)
	if err != nil {
		return err
	}

	// Execute update if needed.
	if err = u.executeUpdateIfNeeded(ctx, versionUpdateNeeded); err != nil {
		return err
	}

	// Start required executables.
	logger.Info(ctx, "Starting required executables")

	if err = u.startRequiredExecutables(ctx); err != nil {
		return fmt.Errorf("start required executables: %w", err)
	}

	return nil
}

// prepareForUpdate handles the initial preparation steps for the update process.
func (u *runner) prepareForUpdate(ctx context.Context) error {
	logger.Info(ctx, "Terminating alarm button processes forcibly")

	if err := u.terminateAlarmButtonProcesses(); err != nil {
		return fmt.Errorf("terminate alarm button processes: %w", err)
	}

	logger.Info(ctx, "Detecting local version from installed executable")

	if err := u.detectAndSetLocalVersion(ctx); err != nil {
		return fmt.Errorf("detect local version: %w", err)
	}

	logger.Info(ctx, "Downloading the update description from the server")

	if err := u.fillUpdateDescription(); err != nil {
		return fmt.Errorf("download update description: %w", err)
	}

	return nil
}

// detectAndSetLocalVersion detects the local version and stores it for later use.
func (u *runner) detectAndSetLocalVersion(ctx context.Context) error {
	localVersion, err := u.detectLocalVersion(ctx)
	if err != nil {
		return err
	}

	u.localVersion = localVersion

	return nil
}

// determineUpdateNeeded checks if an update is required based on version and checksum comparison.
func (u *runner) determineUpdateNeeded(ctx context.Context) (bool, error) {
	remoteVersion := u.description.VersionNumber
	versionUpdateNeeded := u.compareVersions(ctx, u.localVersion, remoteVersion)

	logger.Info(ctx, "Verifying the checksum of files on the client and server")

	if err := u.validateChecksum(); err != nil {
		return false, fmt.Errorf("validate checksum: %w", err)
	}

	return versionUpdateNeeded, nil
}

// executeUpdateIfNeeded performs the update process if either version or file updates are needed.
func (u *runner) executeUpdateIfNeeded(ctx context.Context, versionUpdateNeeded bool) error {
	if !versionUpdateNeeded && !u.IsUpdateNeeded {
		logger.Info(ctx, "No update required - version and files are current")
		return nil
	}

	u.logUpdateReasons(ctx, versionUpdateNeeded)

	logger.Info(ctx, "Downloading update files to a temporary folder")

	if err := u.downloadFiles(ctx); err != nil {
		return fmt.Errorf("download update files: %w", err)
	}

	logger.Info(ctx, "Updating files on the client")

	if err := u.updateFiles(ctx); err != nil {
		return fmt.Errorf("update files on client: %w", err)
	}

	return nil
}

// logUpdateReasons logs the reasons why an update is needed.
func (u *runner) logUpdateReasons(ctx context.Context, versionUpdateNeeded bool) {
	if versionUpdateNeeded {
		logger.InfoKV(ctx, "Version update required", "reason", "version_mismatch")
	}

	if u.IsUpdateNeeded {
		logger.InfoKV(ctx, "File update required", "reason", "checksum_mismatch")
	}
}

// detectLocalVersion runs the appropriate executable to get the current version.
func (u *runner) detectLocalVersion(ctx context.Context) (string, error) {
	var executable string

	switch u.cfg.UpdateType {
	case "client":
		executable = checkerExecutable()
	case "server":
		executable = serverExecutable()
	default:
		return "", fmt.Errorf("%w: %s", errUnknownUpdateType, u.cfg.UpdateType)
	}

	// Create a context with timeout to avoid hanging
	cmdCtx, cancel := context.WithTimeout(ctx, versionCommandTimeout)
	defer cancel()

	// Try to execute: alarm-checker version OR alarm-server version
	cmd := exec.CommandContext(cmdCtx, executable, "version")

	output, err := cmd.Output()
	if err != nil {
		logger.Warnf(ctx, "Could not get local version from %s: %v", executable, err)
		return "", nil // Not an error - might be first install
	}

	// Parse version from output
	return parseVersionFromOutput(string(output))
}

// parseVersionFromOutput extracts semantic version from executable version output.
func parseVersionFromOutput(output string) (string, error) {
	// Parse "version: 1.0.0, commit: abc123, built at: ..." → "1.0.0"
	output = strings.TrimSpace(output)
	if strings.HasPrefix(output, "version: ") {
		parts := strings.Split(output, ",")
		if len(parts) > 0 {
			version := strings.TrimSpace(strings.TrimPrefix(parts[0], "version: "))
			if version != "" {
				return version, nil
			}
		}
	}

	return "", errInvalidVersionOutput
}

// compareVersions compares local vs remote versions and logs the decision.
func (u *runner) compareVersions(ctx context.Context, localVersion, remoteVersion string) bool {
	if localVersion == "" {
		logger.Info(ctx, "No local version detected, update needed")
		return true
	}

	if localVersion != remoteVersion {
		logger.InfoKV(ctx, "Version mismatch detected",
			"local", localVersion, "remote", remoteVersion)

		return true
	}

	logger.InfoKV(ctx, "Versions match, checking file integrity",
		"version", localVersion)

	// Still check checksums for integrity.
	return false
}

// ensureServerReachable verifies that the server is reachable and responsive.
func (u *runner) ensureServerReachable(ctx context.Context) error {
	if u.cfg == nil {
		return errSettingsNotInitialised
	}

	actor, err := common.DetectActor()
	if err != nil {
		return err
	}

	var client *common.Client

	client, err = common.Dial(ctx, u.cfg.ServerAddress, common.WithCallTimeout(u.cfg.Timeout))
	if err != nil {
		return err
	}

	defer func() {
		_ = client.Close()
	}()

	if _, err = client.GetAlarmState(ctx, actor); err != nil {
		return err
	}

	logger.InfoKV(ctx, "Connected to alarm server", "address", u.cfg.ServerAddress)

	return nil
}

// terminateAlarmButtonProcesses kills known binaries before update.
func (u *runner) terminateAlarmButtonProcesses() error {
	executableFiles := sliceToSet(FilesWithChecksum())

	processList, err := ps.Processes()
	if err != nil {
		return err
	}

	thisProcessID := os.Getpid()

	for _, process := range processList {
		processID := process.Pid()
		if processID == thisProcessID {
			continue
		}

		processName := process.Executable()
		if _, found := executableFiles[processName]; !found {
			continue
		}

		var runningProcess *os.Process

		runningProcess, err = os.FindProcess(processID)
		if err != nil {
			return err
		}

		if err = runningProcess.Kill(); err != nil {
			return err
		}
	}

	return nil
}

// fillUpdateDescription downloads and parses the remote update manifest.
func (u *runner) fillUpdateDescription() error {
	response, err := u.getFileBodyFromServer(context.Background(), VersionFilename)
	if response != nil {
		defer func() {
			_ = response.Body.Close()
		}()
	}

	if err != nil {
		return err
	}

	data, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	var desc Description
	if err = yaml.Unmarshal(data, &desc); err != nil {
		return err
	}

	u.description = &desc

	return nil
}

// getFileBodyFromServer fetches a file from the update server folder.
func (u *runner) getFileBodyFromServer(ctx context.Context, fileName string) (*http.Response, error) {
	serverUpdateURL, err := url.Parse(u.cfg.ServerUpdateFolder)
	if err != nil {
		return nil, err
	}

	// Use path.Join to normalize duplicate slashes when composing the URL path.
	serverUpdateURL.Path = path.Join(serverUpdateURL.Path, fileName)
	finalURL := serverUpdateURL.String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, finalURL, http.NoBody)
	if err != nil {
		return nil, err
	}

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return response, err
	}

	if response.StatusCode != http.StatusOK {
		return response, fmt.Errorf("%s, %s: %w", finalURL, response.Status, errBadHTTPStatus)
	}

	return response, err
}

// ValidateChecksum compares local and server checksums to decide whether an update is required.
// It returns early on the first mismatch to avoid unnecessary I/O when an update
// is already known to be needed.
func (u *runner) validateChecksum() error {
	if u.description == nil {
		return errEmptyDescription
	}

	files, ok := u.description.Roles[u.cfg.UpdateType]
	if !ok {
		return fmt.Errorf("role %s: %w", u.cfg.UpdateType, errNoRoleFiles)
	}

	for _, fileName := range files {
		needsUpdate, err := u.validateFileChecksum(fileName)
		if err != nil {
			return err
		}

		if needsUpdate {
			u.IsUpdateNeeded = true
			return nil
		}
	}

	return nil
}

// validateFileChecksum validates a single file's checksum against the server.
// Returns true if the file needs updating, false if it's up to date.
func (u *runner) validateFileChecksum(fileName string) (bool, error) {
	serverChecksum, err := u.getServerChecksum(fileName)
	if err != nil {
		return false, err
	}

	clientChecksum, err := u.getClientChecksum(fileName)
	if err != nil {
		return false, err
	}

	return !bytes.Equal(serverChecksum, clientChecksum), nil
}

// getServerChecksum retrieves and decodes the server checksum for a file.
func (u *runner) getServerChecksum(fileName string) ([]byte, error) {
	serverFileBase64, hasDescription := u.description.Files[fileName]
	if !hasDescription {
		return nil, fmt.Errorf("checksum for %s: %w", fileName, errNoChecksum)
	}

	serverFileChecksum, err := base64.StdEncoding.DecodeString(serverFileBase64)
	if err != nil {
		return nil, err
	}

	return serverFileChecksum, nil
}

// getClientChecksum retrieves the client checksum for a file.
// Returns nil checksum if the file doesn't exist.
func (u *runner) getClientChecksum(fileName string) ([]byte, error) {
	if _, err := os.Stat(fileName); err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, needs update.
			return nil, nil
		}

		return nil, err
	}

	return GetFileChecksum(fileName)
}

// downloadFiles downloads required files into a temporary directory.
func (u *runner) downloadFiles(ctx context.Context) error {
	temporaryDirectory, err := os.MkdirTemp("", "alarm-button-updater-")
	if err != nil {
		return err
	}

	u.temporaryDirectory = temporaryDirectory

	files := u.description.Roles[u.cfg.UpdateType]
	for _, fileName := range files {
		var response *http.Response

		response, err = u.getFileBodyFromServer(ctx, fileName)
		if err != nil {
			if response != nil {
				_ = response.Body.Close()
			}

			return err
		}

		outputFileName := filepath.Clean(filepath.Join(temporaryDirectory, fileName))

		var outputFile *os.File

		outputFile, err = os.Create(outputFileName)
		if err != nil {
			_ = response.Body.Close()

			return err
		}

		_, err = io.Copy(outputFile, response.Body)
		if err != nil {
			_ = response.Body.Close()
			_ = outputFile.Close()

			return err
		}

		u.downloadedFiles[fileName] = outputFileName
		logger.InfoKV(ctx, "Downloaded file", "path", outputFileName)
	}

	return nil
}

// updateFiles applies downloaded files using go-update with checksum validation.
func (u *runner) updateFiles(ctx context.Context) error {
	for fileName, downloadedFileName := range u.downloadedFiles {
		logger.InfoKV(ctx, "Updating file", "file", fileName)

		data, err := os.ReadFile(downloadedFileName)
		if err != nil {
			return err
		}

		logger.Debug(ctx, "Looking for a checksum")

		downloadedFileBase64, ok := u.description.Files[fileName]
		if !ok {
			return fmt.Errorf("checksum for %s: %w", downloadedFileName, errNoChecksum)
		}

		var downloadedFileChecksum []byte

		downloadedFileChecksum, err = base64.StdEncoding.DecodeString(downloadedFileBase64)
		if err != nil {
			return err
		}

		if _, err = os.Stat(fileName); err != nil && os.IsNotExist(err) {
			if _, err = os.Create(fileName); err != nil {
				return err
			}
		}

		logger.Debug(ctx, "Applying update")

		options := &goupdate.Options{
			TargetPath: fileName,
			TargetMode: DefaultFileMode,
			Checksum:   downloadedFileChecksum,
			Hash:       DefaultChecksumFunction,
		}

		dataReader := bytes.NewReader(data)
		if err = goupdate.Apply(dataReader, *options); err != nil {
			return err
		}

		oldFileName := fileName + ".old"
		if _, err = os.Stat(oldFileName); err == nil {
			_ = os.Remove(oldFileName)
		}
	}

	return nil
}

// startRequiredExecutables launches the role-specific binary according to the manifest.
func (u *runner) startRequiredExecutables(ctx context.Context) error {
	if u.description == nil {
		return errEmptyDescription
	}

	executable, ok := u.description.Executables[u.cfg.UpdateType]
	if !ok {
		return fmt.Errorf("role %s: %w", u.cfg.UpdateType, errNoRoleExecutable)
	}

	logger.InfoKV(ctx, "Starting executable", "executable", executable)

	osLC := strings.ToLower(runtime.GOOS)
	switch {
	case strings.Contains(osLC, "linux") || strings.Contains(osLC, "darwin"):
		return exec.CommandContext(ctx, executable).Start()
	case strings.Contains(osLC, "windows"):
		return exec.CommandContext(ctx, "cmd.exe", "/C", "start", executable).Start()
	default:
		return fmt.Errorf("%s OS is not supported: %w", runtime.GOOS, errUnsupportedOS)
	}
}

// cleanup removes temporary artifacts and the running marker.
func (u *runner) cleanup(ctx context.Context) {
	if _, err := os.Stat(MarkerFilename); err == nil {
		_ = os.Remove(MarkerFilename)
	}

	if u.temporaryDirectory != "" {
		if _, err := os.Stat(u.temporaryDirectory); err == nil {
			_ = os.RemoveAll(u.temporaryDirectory)
		}
	}

	logger.Info(ctx, "The updater has been stopped")
}
