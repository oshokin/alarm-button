package updater

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRoleSpecs_DoNotContainConfigAsBinary verifies config file is not in binary artifact list.
func TestRoleSpecs_DoNotContainConfigAsBinary(t *testing.T) {
	t.Parallel()

	for _, role := range RoleSpecsForPlatform(runtime.GOOS) {
		require.NotNil(t, role)
		require.NotContains(t, role.Files, "alarm-button-settings.yaml")
		require.NotEmpty(t, role.PIDFile)
	}
}

// TestTargetPathForArtifact_ConfigPathIsFixed verifies config artifact target path behavior.
func TestTargetPathForArtifact_ConfigPathIsFixed(t *testing.T) {
	t.Parallel()

	artifact := stagedArtifact{
		Name: "alarm-button-settings-client.yaml",
		Kind: ArtifactKindConfig,
	}
	configPath := filepath.Join(t.TempDir(), "alarm-button-settings.yaml")
	testRunner := &runner{
		installRoot: t.TempDir(),
		configPath:  configPath,
	}
	path, err := testRunner.targetPathForArtifact(&artifact)
	require.NoError(t, err)
	require.Equal(t, filepath.Clean(configPath), path)
}
