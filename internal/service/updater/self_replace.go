package updater

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// Windows self-replacement helper file permissions.
const (
	selfReplaceScriptFileMode = 0o600
	selfReplaceBinaryFileMode = 0o755
)

// scheduleWindowsUpdaterReplacement launches detached helper for self-update handoff.
func scheduleWindowsUpdaterReplacement(
	updaterPID int,
	stagedPath string,
	targetPath string,
) error {
	suffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
	handoffPath := targetPath + ".self-update." + suffix + ".new"
	scriptPath := targetPath + ".self-update." + suffix + ".ps1"

	if err := copyFileForSelfReplacement(stagedPath, handoffPath); err != nil {
		return err
	}

	scriptText := []byte(`param(
  [int]$UpdaterPid,
  [string]$SourcePath,
  [string]$TargetPath
)
$ErrorActionPreference = "Stop"
for ($i = 0; $i -lt 300; $i++) {
  if (-not (Get-Process -Id $UpdaterPid -ErrorAction SilentlyContinue)) {
    break
  }

  Start-Sleep -Milliseconds 200
}

for ($i = 0; $i -lt 300; $i++) {
  try {
    Move-Item -LiteralPath $SourcePath -Destination $TargetPath -Force
    Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
    exit 0
  } catch {
    Start-Sleep -Milliseconds 200
  }
}

exit 1
`)
	if err := os.WriteFile(scriptPath, scriptText, selfReplaceScriptFileMode); err != nil {
		_ = os.Remove(handoffPath)
		return fmt.Errorf("write self replacement script: %w", err)
	}

	command := exec.CommandContext(
		context.Background(),
		"powershell.exe",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy",
		"Bypass",
		"-File",
		scriptPath,
		"-UpdaterPid",
		strconv.Itoa(updaterPID),
		"-SourcePath",
		handoffPath,
		"-TargetPath",
		targetPath,
	)

	if err := command.Start(); err != nil {
		_ = os.Remove(scriptPath)
		_ = os.Remove(handoffPath)

		return fmt.Errorf("start self replacement helper: %w", err)
	}

	if command.Process != nil {
		releaseErr := command.Process.Release()
		_ = releaseErr // Helper already started; release failure is non-fatal.
	}

	return nil
}

// copyFileForSelfReplacement copies staged updater binary into handoff location.
func copyFileForSelfReplacement(sourcePath, targetPath string) error {
	input, err := os.Open(filepath.Clean(sourcePath))
	if err != nil {
		return fmt.Errorf("open self replacement source: %w", err)
	}
	defer func() { _ = input.Close() }()

	output, err := os.OpenFile(
		filepath.Clean(targetPath),
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		selfReplaceBinaryFileMode,
	)
	if err != nil {
		return fmt.Errorf("open self replacement target: %w", err)
	}

	_, copyErr := io.Copy(output, input)
	if syncErr := output.Sync(); syncErr != nil && copyErr == nil {
		copyErr = syncErr
	}

	if closeErr := output.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}

	if copyErr != nil {
		return fmt.Errorf("copy self replacement source: %w", copyErr)
	}

	return nil
}
