package packager

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/logger"
	"github.com/oshokin/alarm-button/internal/service/common"
	"github.com/oshokin/alarm-button/internal/service/updater"
)

// Options contains inputs for the packager entry point.
type Options struct {
	// ConfigPath is an optional path to persist connection settings (defaults to settings.yaml).
	ConfigPath string
	// ServerAddress is the gRPC address of the running alarm-server used for reachability checks.
	ServerAddress string
	// UpdateFolder is the URL or path where update artifacts will be uploaded.
	UpdateFolder string
}

// packager prepares update metadata (manifest) for distribution.
// It is unexported—callers should use Run, which encapsulates setup and validation.
type packager struct {
	// cfg holds the configuration for server connection and update folder.
	cfg *config.Config
	// cfgFilename is the path where configuration is saved.
	cfgFilename string
	// desc contains the update manifest with files, roles, and executables.
	desc *updater.Description
}

// errUpdaterRunning indicates that an attempt was made to start the updater while it is already running.
var errUpdaterRunning = errors.New("the updater is running now")

// Run executes the packaging workflow.
func Run(ctx context.Context, opts *Options) error {
	// Set context with logger name for tracking.
	ctx = logger.WithName(ctx, "alarm-packager")

	cfg := &config.Config{
		ServerAddress:      opts.ServerAddress,
		ServerUpdateFolder: opts.UpdateFolder,
	}
	if err := config.Validate(cfg); err != nil {
		return err
	}

	pkg, err := newPackager(ctx, opts.ConfigPath, cfg)
	if err != nil {
		return fmt.Errorf("initialize packager: %w", err)
	}

	if err = pkg.Run(ctx); err != nil {
		return fmt.Errorf("packager failed: %w", err)
	}

	logger.Info(ctx, "Packager completed successfully")

	return nil
}

// newPackager creates a new packager instance with the provided settings and configuration path.
func newPackager(ctx context.Context, configFilename string, settings *config.Config) (*packager, error) {
	if updater.IsUpdaterRunningNow(ctx) {
		return nil, errUpdaterRunning
	}

	if err := config.Save(configFilename, settings); err != nil {
		return nil, fmt.Errorf("save settings: %w", err)
	}

	pkg := &packager{
		cfg:         settings,
		cfgFilename: configFilename,
		desc:        updater.NewDescription(),
	}

	if err := pkg.ensureServerReachable(ctx); err != nil {
		return nil, err
	}

	return pkg, nil
}

// Run populates and writes the update description (manifest) to disk.
func (p *packager) Run(ctx context.Context) error {
	logger.Info(ctx, "Preparing update description")

	if err := p.fillDescription(); err != nil {
		return err
	}

	logger.InfoKV(ctx, "Saving update description", "path", updater.VersionFilename)

	if err := p.saveDescription(); err != nil {
		return err
	}

	p.printNextSteps(ctx)

	return nil
}

// fillDescription populates roles, executables and file checksums into the manifest.
func (p *packager) fillDescription() error {
	for role, files := range updater.AllowedUserRoles() {
		p.desc.Roles[role] = append([]string(nil), files...)
	}

	maps.Copy(p.desc.Executables, updater.ExecutablesByUserRoles())

	for _, fileName := range updater.FilesWithChecksum() {
		if _, err := os.Stat(fileName); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: %w", fileName, os.ErrNotExist)
		} else if err != nil {
			return fmt.Errorf("stat %s: %w", fileName, err)
		}

		checksum, err := updater.GetFileChecksum(fileName)
		if err != nil {
			return err
		}

		p.desc.Files[fileName] = base64.StdEncoding.EncodeToString(checksum)
	}

	return nil
}

// saveDescription writes the manifest to the standard VersionFilename.
func (p *packager) saveDescription() error {
	contents, err := yaml.Marshal(p.desc)
	if err != nil {
		return err
	}

	return os.WriteFile(updater.VersionFilename, contents, updater.DefaultFileMode)
}

// printNextSteps logs human-readable guidance for next actions with the created files.
func (p *packager) printNextSteps(ctx context.Context) {
	files := make([]string, 0, len(p.desc.Files)+1)
	for fileName := range p.desc.Files {
		files = append(files, fileName)
	}

	files = append(files, updater.VersionFilename)
	sort.Strings(files)

	var builder strings.Builder

	builder.WriteString("You should upload the following files to the folder ")
	builder.WriteString(p.cfg.ServerUpdateFolder)
	builder.WriteString(":\n")

	for i, name := range files {
		if i == 0 {
			builder.WriteString(name)
		} else {
			builder.WriteString(",\n")
			builder.WriteString(name)
		}
	}

	for role, fileList := range p.desc.Roles {
		builder.WriteString("\n\nFor a user with the \"")
		builder.WriteString(role)
		builder.WriteString("\" role, copy the following files to the local computer:\n")

		for i, name := range fileList {
			if i == 0 {
				builder.WriteString(name)
			} else {
				builder.WriteString(",\n")
				builder.WriteString(name)
			}
		}

		builder.WriteString("\nAt system startup, set the command to run: alarm-updater ")
		builder.WriteString(role)
	}

	logger.Info(ctx, builder.String())
}

// ensureServerReachable verifies that the server is reachable before generating a manifest.
func (p *packager) ensureServerReachable(ctx context.Context) error {
	actor, err := common.DetectActor()
	if err != nil {
		return err
	}

	var client *common.Client

	client, err = common.Dial(ctx, p.cfg.ServerAddress, common.WithCallTimeout(p.cfg.Timeout))
	if err != nil {
		return err
	}

	// Best-effort cleanup.
	defer func() {
		_ = client.Close()
	}()

	if _, err = client.GetAlarmState(ctx, actor); err != nil {
		return err
	}

	logger.InfoKV(ctx, "Verified connection to alarm server", "server_address", p.cfg.ServerAddress)

	return nil
}
