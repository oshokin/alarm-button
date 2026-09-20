//go:build windows

package integration

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/oshokin/alarm-button/internal/config"
	"github.com/oshokin/alarm-button/internal/service/packager"
	"github.com/oshokin/alarm-button/internal/service/updater"
)

// Helper environment variable names used by subprocess-driven E2E scenario.
const (
	// updaterE2EHelperEnv enables helper process path for updater invocation.
	updaterE2EHelperEnv = "ALARM_UPDATER_E2E_HELPER"
	// updaterE2EInstallRootEnv provides install root path for helper process.
	updaterE2EInstallRootEnv = "ALARM_UPDATER_E2E_INSTALL_ROOT"
	// updaterE2EConfigPathEnv provides config path for helper process.
	updaterE2EConfigPathEnv = "ALARM_UPDATER_E2E_CONFIG_PATH"
	// updaterE2EKeyIDEnv provides expected manifest key identifier for helper process.
	updaterE2EKeyIDEnv = "ALARM_UPDATER_E2E_KEY_ID"
	// updaterE2EPublicKeyEnv provides trusted public key for helper process.
	updaterE2EPublicKeyEnv = "ALARM_UPDATER_E2E_PUBLIC_KEY"
)

// updaterBuildTimeout bounds compilation time for fixture binaries.
// Time budget for compiling fixture binaries in E2E test.
const updaterBuildTimeout = 2 * time.Minute

// TestUpdater_WindowsSelfReplaceEndToEnd validates deferred updater replacement and no-op second run.
//
//nolint:cyclop,gocognit,funlen // Windows acceptance scenario intentionally validates full updater lifecycle.
func TestUpdater_WindowsSelfReplaceEndToEnd(t *testing.T) {
	rootDir := t.TempDir()
	installRoot := filepath.Join(rootDir, "install")
	inputDir := filepath.Join(rootDir, "input")
	repositoryDir := filepath.Join(rootDir, "repository")

	require.NoError(t, os.MkdirAll(installRoot, 0o755))
	require.NoError(t, os.MkdirAll(inputDir, 0o755))
	require.NoError(t, os.MkdirAll(repositoryDir, 0o755))

	specs := updater.RoleSpecsForPlatform(runtime.GOOS)
	clientSpec := specs[updater.RoleClient]
	artifactNames := sortedUniqueArtifacts(specs)

	checkerBinary := filepath.Join(rootDir, "checker.exe")
	oldUpdaterBinary := filepath.Join(rootDir, "updater-old.exe")
	newUpdaterBinary := filepath.Join(rootDir, "updater-new.exe")
	noopBinary := filepath.Join(rootDir, "noop.exe")

	buildFixtureBinary(t, checkerBinary, checkerFixtureSource())
	buildFixtureBinary(t, oldUpdaterBinary, updaterFixtureSource("1.0.0"))
	buildFixtureBinary(t, newUpdaterBinary, updaterFixtureSource("2.0.0"))
	buildFixtureBinary(t, noopBinary, noopFixtureSource)

	updaterArtifactName := "alarm-updater" + executableSuffixWindows

	for _, artifactName := range artifactNames {
		installTarget := filepath.Join(installRoot, artifactName)
		inputTarget := filepath.Join(inputDir, artifactName)

		switch artifactName {
		case clientSpec.Executable:
			copyFixtureBinary(t, checkerBinary, installTarget)
			copyFixtureBinary(t, checkerBinary, inputTarget)
		case updaterArtifactName:
			copyFixtureBinary(t, oldUpdaterBinary, installTarget)
			copyFixtureBinary(t, newUpdaterBinary, inputTarget)
		default:
			copyFixtureBinary(t, noopBinary, installTarget)
			copyFixtureBinary(t, noopBinary, inputTarget)
		}
	}

	server := httptest.NewServer(http.FileServer(http.Dir(repositoryDir)))
	defer server.Close()

	configPayload := []byte(fmt.Sprintf(
		"server_addr: 127.0.0.1:50051\nlisten_addr: 127.0.0.1:50051\nupdate_folder: %s\ntimeout: 2s\n",
		server.URL,
	))
	configPath := filepath.Join(installRoot, config.DefaultConfigFilename)
	require.NoError(t, os.WriteFile(configPath, configPayload, 0o600))

	clientConfigPath := filepath.Join(rootDir, "client.yaml")
	serverConfigPath := filepath.Join(rootDir, "server.yaml")
	require.NoError(t, os.WriteFile(clientConfigPath, configPayload, 0o600))
	require.NoError(t, os.WriteFile(serverConfigPath, configPayload, 0o600))

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	pkcs8PrivateKey, err := x509.MarshalPKCS8PrivateKey(privateKey)
	require.NoError(t, err)

	privateKeyPath := filepath.Join(rootDir, "signing-key.pem")
	require.NoError(
		t,
		os.WriteFile(
			privateKeyPath,
			pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8PrivateKey}),
			0o600,
		),
	)

	const keyID = "windows-e2e-key"

	err = packager.Run(&packager.Options{
		InputDir:             inputDir,
		OutputDir:            repositoryDir,
		Version:              "2.0.0",
		SigningKey:           privateKeyPath,
		KeyID:                keyID,
		GOOS:                 runtime.GOOS,
		GOARCH:               runtime.GOARCH,
		ClientConfig:         clientConfigPath,
		ClientConfigRevision: 2,
		ServerConfig:         serverConfigPath,
		ServerConfigRevision: 2,
	})
	require.NoError(t, err)

	updaterPath := filepath.Join(installRoot, updaterArtifactName)
	require.Equal(t, "1.0.0", runFixtureVersion(t, updaterPath))

	runUpdaterE2EHelper(t, installRoot, configPath, keyID, publicKey)

	expectedUpdaterChecksum, err := checksumFileBase64(newUpdaterBinary)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		currentChecksum, checksumErr := checksumFileBase64(updaterPath)
		if checksumErr != nil {
			return false
		}

		return currentChecksum == expectedUpdaterChecksum
	}, 40*time.Second, 200*time.Millisecond)

	require.Equal(t, "2.0.0", runFixtureVersion(t, updaterPath))

	secondRunOutput := runUpdaterE2EHelper(t, installRoot, configPath, keyID, publicKey)
	require.Contains(t, secondRunOutput, "No update required")

	t.Cleanup(func() {
		terminateProcessFromPIDFile(filepath.Join(installRoot, clientSpec.PIDFile))
	})
}

// TestUpdater_WindowsSelfReplaceEndToEnd_Helper runs updater once inside helper subprocess.
func TestUpdater_WindowsSelfReplaceEndToEnd_Helper(t *testing.T) {
	if os.Getenv(updaterE2EHelperEnv) != "1" {
		t.Skip("helper process only")
	}

	installRoot := os.Getenv(updaterE2EInstallRootEnv)
	configPath := os.Getenv(updaterE2EConfigPathEnv)
	keyID := os.Getenv(updaterE2EKeyIDEnv)
	publicKeyBase64 := os.Getenv(updaterE2EPublicKeyEnv)
	require.NotEmpty(t, installRoot)
	require.NotEmpty(t, configPath)
	require.NotEmpty(t, keyID)
	require.NotEmpty(t, publicKeyBase64)

	publicKeyRaw, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	require.NoError(t, err)

	err = updater.Run(context.Background(), &updater.Options{
		ConfigPath:  configPath,
		UpdateType:  string(updater.RoleClient),
		InstallRoot: installRoot,
		TrustedManifestKeys: map[string]ed25519.PublicKey{
			keyID: ed25519.PublicKey(publicKeyRaw),
		},
	})
	require.NoError(t, err)
}

// runUpdaterE2EHelper executes updater run in separate process to allow self-replacement.
func runUpdaterE2EHelper(
	t *testing.T,
	installRoot string,
	configPath string,
	keyID string,
	publicKey ed25519.PublicKey,
) string {
	t.Helper()

	testExecutable, err := os.Executable()
	require.NoError(t, err)

	command := exec.Command(testExecutable, "-test.run", "^TestUpdater_WindowsSelfReplaceEndToEnd_Helper$")
	command.Env = append(
		os.Environ(),
		updaterE2EHelperEnv+"=1",
		updaterE2EInstallRootEnv+"="+installRoot,
		updaterE2EConfigPathEnv+"="+configPath,
		updaterE2EKeyIDEnv+"="+keyID,
		updaterE2EPublicKeyEnv+"="+base64.StdEncoding.EncodeToString(publicKey),
	)

	output, err := command.CombinedOutput()
	require.NoErrorf(t, err, "updater helper run failed: %s", string(output))

	return string(output)
}

// sortedUniqueArtifacts returns deterministic unique artifact list from role specs.
func sortedUniqueArtifacts(specs map[updater.Role]*updater.RoleSpec) []string {
	unique := make(map[string]struct{})
	for _, spec := range specs {
		if spec == nil {
			continue
		}

		for _, name := range spec.Files {
			unique[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(unique))
	for name := range unique {
		names = append(names, name)
	}

	sort.Strings(names)
	return names
}

// buildFixtureBinary compiles fixture Go source into executable file.
func buildFixtureBinary(t *testing.T, outputPath string, source string) {
	t.Helper()

	sourcePath := filepath.Join(t.TempDir(), filepath.Base(outputPath)+".go")
	require.NoError(t, os.WriteFile(sourcePath, []byte(source), 0o600))

	ctx, cancel := context.WithTimeout(context.Background(), updaterBuildTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, "go", "build", "-o", outputPath, sourcePath)
	output, err := command.CombinedOutput()
	require.NoErrorf(t, err, "build fixture binary failed: %s", string(output))
}

// copyFixtureBinary copies compiled fixture executable to destination path.
func copyFixtureBinary(t *testing.T, sourcePath string, targetPath string) {
	t.Helper()

	content, err := os.ReadFile(filepath.Clean(sourcePath))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(targetPath, content, 0o755))
}

// runFixtureVersion executes fixture binary version command.
func runFixtureVersion(t *testing.T, executablePath string) string {
	t.Helper()

	output, err := exec.Command(executablePath, "version", "--short").CombinedOutput()
	require.NoErrorf(t, err, "version command failed: %s", string(output))

	return strings.TrimSpace(string(output))
}

// terminateProcessFromPIDFile attempts to stop process referenced by PID file.
func terminateProcessFromPIDFile(pidPath string) {
	content, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(content)))
	if err != nil {
		return
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return
	}

	_ = process.Kill()
}

// checkerFixtureSource returns synthetic checker binary source for E2E test.
func checkerFixtureSource() string {
	return `package main
import (
  "os"
  "path/filepath"
  "strconv"
  "time"
)

func main() {
  if len(os.Args) >= 3 && os.Args[1] == "version" && os.Args[2] == "--short" {
    _, _ = os.Stdout.WriteString("1.0.0\n")
    return
  }

  configPath := ""
  for i := 1; i+1 < len(os.Args); i++ {
    if os.Args[i] == "--config" {
      configPath = os.Args[i+1]
      break
    }
  }

  if configPath == "" {
    return
  }

  pidFile := filepath.Join(filepath.Dir(configPath), "alarm-checker.pid")
  _ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
  defer os.Remove(pidFile)

  time.Sleep(15 * time.Second)
}
`
}

// updaterFixtureSource returns synthetic updater binary source with fixed version output.
func updaterFixtureSource(version string) string {
	return fmt.Sprintf(`package main
import (
  "os"
)

func main() {
  if len(os.Args) >= 3 && os.Args[1] == "version" && os.Args[2] == "--short" {
    _, _ = os.Stdout.WriteString(%q + "\n")
  }
}
`, version)
}

// noopFixtureSource is minimal executable source for unrelated role artifacts.
const noopFixtureSource = `package main
func main() {}
`

// executableSuffixWindows is executable file extension used by test fixtures.
const executableSuffixWindows = ".exe"
