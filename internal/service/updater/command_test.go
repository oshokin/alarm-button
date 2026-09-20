package updater

import (
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/proc"
)

// TestResolveConfigPath_DefaultUsesInstallRoot checks default config path resolution.
func TestResolveConfigPath_DefaultUsesInstallRoot(t *testing.T) {
	t.Parallel()

	installRoot := t.TempDir()
	configPath, err := resolveConfigPath("", installRoot)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(installRoot, "alarm-button-settings.yaml"), configPath)

	configPath, err = resolveConfigPath("alarm-button-settings.yaml", installRoot)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(installRoot, "alarm-button-settings.yaml"), configPath)
}

// TestResolveConfigPath_CustomRelativePathUsesAbsolutePath checks relative custom path handling.
func TestResolveConfigPath_CustomRelativePathUsesAbsolutePath(t *testing.T) {
	t.Parallel()

	installRoot := t.TempDir()
	configPath, err := resolveConfigPath("./custom/settings.yaml", installRoot)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(configPath))
}

// TestTrustedManifestKeysForOptions_UsesOverrideMapCopy checks trusted key map defensive copy.
func TestTrustedManifestKeysForOptions_UsesOverrideMapCopy(t *testing.T) {
	t.Parallel()

	originalKey := ed25519.PublicKey("public-key")
	options := &Options{
		TrustedManifestKeys: map[string]ed25519.PublicKey{
			"test": originalKey,
		},
	}

	keys := trustedManifestKeysForOptions(options)
	require.NotNil(t, keys)
	require.Equal(t, ed25519.PublicKey("public-key"), keys["test"])

	keys["test"][0] = 'x'

	require.Equal(t, ed25519.PublicKey("public-key"), options.TrustedManifestKeys["test"])
}

// TestResolveDesiredVersion_BinaryManifestUsesManifestVersion checks binary update version behavior.
func TestResolveDesiredVersion_BinaryManifestUsesManifestVersion(t *testing.T) {
	t.Parallel()

	testRunner := &runner{
		manifest: &Manifest{Version: "1.8.0"},
		state:    &UpdateState{ApplicationVersion: "1.9.0"},
	}
	version := testRunner.resolveDesiredVersion("2.0.0", true)
	require.Equal(t, "1.8.0", version)
}

// TestResolveDesiredVersion_ConfigOnlyUsesLocalVersion checks config-only local version retention.
func TestResolveDesiredVersion_ConfigOnlyUsesLocalVersion(t *testing.T) {
	t.Parallel()

	testRunner := &runner{
		manifest: &Manifest{Version: "1.8.0"},
		state:    &UpdateState{ApplicationVersion: "1.9.0"},
	}
	version := testRunner.resolveDesiredVersion("2.0.0", false)
	require.Equal(t, "2.0.0", version)
}

// TestResolveDesiredVersion_ConfigOnlyFallsBackToStateVersion checks state fallback version behavior.
func TestResolveDesiredVersion_ConfigOnlyFallsBackToStateVersion(t *testing.T) {
	t.Parallel()

	testRunner := &runner{
		manifest: &Manifest{Version: "1.8.0"},
		state:    &UpdateState{ApplicationVersion: "1.9.0"},
	}
	version := testRunner.resolveDesiredVersion("", false)
	require.Equal(t, "1.9.0", version)
}

// TestPlanUpdate_ConfigOnlyDoesNotRejectBinaryDowngrade checks config-only downgrade policy bypass.
func TestPlanUpdate_ConfigOnlyDoesNotRejectBinaryDowngrade(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("version probe fixture uses shell script")
	}

	t.Parallel()

	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	installRoot := t.TempDir()
	versionProbeScript := `#!/bin/sh
if [ "$1" = "version" ] && [ "$2" = "--short" ]; then
  echo 2.0.0
  exit 0
fi
exit 0
`
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(installRoot, spec.Executable),
			[]byte(versionProbeScript),
			0o755,
		),
	)

	testRunner := &runner{
		role:        RoleClient,
		spec:        spec,
		installRoot: installRoot,
		configPath:  filepath.Join(installRoot, "alarm-button-settings.yaml"),
		state:       &UpdateState{},
		manifest: &Manifest{
			Version: "1.9.0",
			Configuration: map[Role]*ConfigArtifact{
				RoleClient: {
					Revision: 1,
					Artifact: spec.ConfigArtifact,
					Size:     1,
					SHA512:   validSHA512(),
				},
			},
		},
	}

	plan, err := testRunner.planUpdate(context.Background())
	require.NoError(t, err)
	require.Equal(t, "2.0.0", plan.DesiredVersion)
}

// TestCollectApplyTargets_DefersUpdaterReplacementOnWindowsSpec checks deferred updater target routing.
func TestCollectApplyTargets_DefersUpdaterReplacementOnWindowsSpec(t *testing.T) {
	t.Parallel()

	installRoot := t.TempDir()
	testRunner := &runner{
		spec: &RoleSpec{
			Executable: "alarm-checker.exe",
		},
		installRoot: installRoot,
		configPath:  filepath.Join(installRoot, "alarm-button-settings.yaml"),
	}

	plan := &updatePlan{
		Binaries: []*stagedArtifact{
			{Name: "alarm-checker.exe", Kind: ArtifactKindExecutable},
			{Name: "alarm-updater.exe", Kind: ArtifactKindExecutable},
		},
	}

	targets, deferredUpdater, err := testRunner.collectApplyTargets(
		plan,
		[]string{"alarm-checker.exe", "alarm-updater.exe"},
	)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Equal(t, "alarm-checker.exe", targets[0].Name)
	require.NotNil(t, deferredUpdater)
	require.Equal(t, "alarm-updater.exe", deferredUpdater.ArtifactName)
	require.Equal(t, filepath.Join(installRoot, "alarm-updater.exe"), deferredUpdater.TargetPath)
}

// TestCheckCheckerReadiness_ReadyWhenPIDFileMatchesRunningProcess checks checker readiness success path.
func TestCheckCheckerReadiness_ReadyWhenPIDFileMatchesRunningProcess(t *testing.T) {
	t.Parallel()

	installRoot := t.TempDir()
	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	testRunner := &runner{
		spec:        spec,
		installRoot: installRoot,
	}

	pidPath := filepath.Join(installRoot, spec.PIDFile)
	require.NoError(t, proc.Write(pidPath, os.Getpid()))

	ready, err := testRunner.checkCheckerReadiness(os.Getpid())
	require.NoError(t, err)
	require.True(t, ready)
}

// TestCheckCheckerReadiness_FailsOnPIDMismatch checks checker readiness PID ownership guard.
func TestCheckCheckerReadiness_FailsOnPIDMismatch(t *testing.T) {
	t.Parallel()

	installRoot := t.TempDir()
	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	testRunner := &runner{
		spec:        spec,
		installRoot: installRoot,
	}

	pidPath := filepath.Join(installRoot, spec.PIDFile)
	require.NoError(t, proc.Write(pidPath, os.Getpid()))

	ready, err := testRunner.checkCheckerReadiness(os.Getpid() + 1)
	require.Error(t, err)
	require.False(t, ready)
	require.ErrorIs(t, err, errCheckerPIDMismatch)
}

// TestApplyTransaction_ReadinessFailureDoesNotScheduleUpdaterReplacement checks rollback scheduling safety.
func TestApplyTransaction_ReadinessFailureDoesNotScheduleUpdaterReplacement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("script-based fixture is unix-only")
	}

	installRoot := t.TempDir()
	backupDir := filepath.Join(installRoot, "backup")
	require.NoError(t, os.MkdirAll(backupDir, 0o700))

	configPath := filepath.Join(installRoot, config.DefaultConfigFilename)
	require.NoError(
		t,
		os.WriteFile(configPath, []byte(`
server_addr: 127.0.0.1:50051
listen_addr: 127.0.0.1:50051
update_folder: https://updates.example.com/client
timeout: 2s
`), 0o600),
	)

	oldCheckerPath := filepath.Join(installRoot, "alarm-checker.exe")
	oldUpdaterPath := filepath.Join(installRoot, "alarm-updater.exe")

	require.NoError(t, writeExecutableFixture(oldCheckerPath, []byte("#!/bin/sh\nexit 0\n")))
	require.NoError(t, writeExecutableFixture(oldUpdaterPath, []byte("old-updater\n")))

	stagedDir := filepath.Join(installRoot, "staged")
	require.NoError(t, os.MkdirAll(stagedDir, 0o700))

	stagedCheckerPath := filepath.Join(stagedDir, "alarm-checker.exe")
	stagedUpdaterPath := filepath.Join(stagedDir, "alarm-updater.exe")

	require.NoError(
		t,
		writeExecutableFixture(
			stagedCheckerPath,
			[]byte("#!/bin/sh\necho 1 > alarm-checker.pid\nsleep 5\n"),
		),
	)
	require.NoError(t, writeExecutableFixture(stagedUpdaterPath, []byte("new-updater\n")))

	checksumRunner := &runner{}

	checkerChecksum, err := checksumRunner.checksumPathBase64(stagedCheckerPath)
	require.NoError(t, err)

	updaterChecksum, err := checksumRunner.checksumPathBase64(stagedUpdaterPath)
	require.NoError(t, err)

	testRunner := &runner{
		role: RoleClient,
		spec: &RoleSpec{
			Name:       RoleClient,
			Files:      []string{"alarm-checker.exe", "alarm-updater.exe"},
			Executable: "alarm-checker.exe",
			PIDFile:    "alarm-checker.pid",
		},
		installRoot: installRoot,
		configPath:  configPath,
		cfg: &config.Config{
			Timeout: time.Second,
		},
	}

	plan := &updatePlan{
		Binaries: []*stagedArtifact{
			{
				Name:       "alarm-checker.exe",
				Kind:       ArtifactKindExecutable,
				StagedPath: stagedCheckerPath,
				SHA512:     checkerChecksum,
			},
			{
				Name:       "alarm-updater.exe",
				Kind:       ArtifactKindExecutable,
				StagedPath: stagedUpdaterPath,
				SHA512:     updaterChecksum,
			},
		},
	}

	scheduleCalled := false
	originalSchedule := scheduleUpdaterReplacement
	scheduleUpdaterReplacement = func(_ int, _, _ string) error {
		scheduleCalled = true
		return nil
	}

	t.Cleanup(func() {
		scheduleUpdaterReplacement = originalSchedule
	})

	err = testRunner.applyTransaction(context.Background(), backupDir, plan)
	require.Error(t, err)
	require.ErrorIs(t, err, errCheckerPIDMismatch)
	require.False(t, scheduleCalled, "self-replace helper must not be scheduled on failed readiness")

	helperFiles, globErr := filepath.Glob(filepath.Join(installRoot, "alarm-updater.exe.self-update.*"))
	require.NoError(t, globErr)
	require.Empty(t, helperFiles)
}

// writeExecutableFixture creates executable fixture file for updater tests.
func writeExecutableFixture(path string, content []byte) error {
	return os.WriteFile(path, content, 0o755)
}
