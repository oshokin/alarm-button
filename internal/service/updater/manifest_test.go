package updater

import (
	"encoding/base64"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestManifestValidate verifies valid manifest passes full validation.
func TestManifestValidate(t *testing.T) {
	t.Parallel()

	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	manifest := validManifestForSpec(spec)
	require.NoError(t, manifest.Validate(spec))
}

// TestManifestValidate_RejectsUnexpectedArtifact verifies unknown artifact rejection.
func TestManifestValidate_RejectsUnexpectedArtifact(t *testing.T) {
	t.Parallel()

	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	manifest := validManifestForSpec(spec)
	manifest.Artifacts["../evil"] = &Artifact{
		Kind:   ArtifactKindExecutable,
		Size:   10,
		SHA512: validSHA512(),
	}

	require.Error(t, manifest.Validate(spec))
}

// TestManifestValidate_RejectsWrongConfigArtifactName verifies role config artifact enforcement.
func TestManifestValidate_RejectsWrongConfigArtifactName(t *testing.T) {
	t.Parallel()

	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	manifest := validManifestForSpec(spec)
	cfg := manifest.Configuration[RoleClient]
	cfg.Artifact = "alarm-button-settings-server.yaml"

	require.Error(t, manifest.Validate(spec))
}

// TestManifestValidate_RejectsWrongPlatform verifies platform mismatch rejection.
func TestManifestValidate_RejectsWrongPlatform(t *testing.T) {
	t.Parallel()

	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	manifest := validManifestForSpec(spec)
	manifest.Platform.GOOS = "plan9"

	require.Error(t, manifest.Validate(spec))
}

// TestManifestValidate_AllowsConfigOnlyManifest verifies config-only manifest support.
func TestManifestValidate_AllowsConfigOnlyManifest(t *testing.T) {
	t.Parallel()

	spec, err := localRoleSpec(RoleClient)
	require.NoError(t, err)

	manifest := validManifestForSpec(spec)
	manifest.Artifacts = map[string]*Artifact{}

	require.NoError(t, manifest.Validate(spec))
}

// validManifestForSpec builds minimal valid manifest fixture for role spec.
func validManifestForSpec(spec *RoleSpec) *Manifest {
	artifacts := make(map[string]*Artifact, len(spec.Files))
	for _, name := range spec.Files {
		artifacts[name] = &Artifact{
			Kind:   ArtifactKindExecutable,
			Size:   123,
			SHA512: validSHA512(),
		}
	}

	return &Manifest{
		SchemaVersion: CurrentManifestSchema,
		Version:       "1.8.0",
		Platform: Platform{
			GOOS:   runtime.GOOS,
			GOARCH: runtime.GOARCH,
		},
		Artifacts: artifacts,
		Configuration: map[Role]*ConfigArtifact{
			spec.Name: {
				Revision: 1,
				Artifact: spec.ConfigArtifact,
				Size:     100,
				SHA512:   validSHA512(),
			},
		},
	}
}

// validSHA512 returns base64 placeholder SHA-512 digest for tests.
func validSHA512() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 64))
}
