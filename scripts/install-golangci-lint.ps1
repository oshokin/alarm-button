$ErrorActionPreference = "Stop"

$golangciTag = "v$($env:GOLANGCI_VERSION)"

if ($env:GOLANGCI_CURRENT_TAG -eq $golangciTag) {
    exit 0
}

if (Test-Path $env:GOLANGCI_BIN) {
    $versionOutput = (& $env:GOLANGCI_BIN version 2>$null | Out-String)
    if ($versionOutput -match "has version (v?[0-9]+\.[0-9]+\.[0-9]+)") {
        $installedTag = $matches[1]
        if (-not $installedTag.StartsWith("v")) {
            $installedTag = "v$installedTag"
        }
        if ($installedTag -eq $golangciTag) {
            exit 0
        }
    }
}

Write-Host "Installing golangci-lint, version: $golangciTag..."

$goArch = (& go env GOARCH).Trim()

switch ($goArch) {
    "amd64" { $archive = "golangci-lint-$($env:GOLANGCI_VERSION)-windows-amd64.zip" }
    "arm64" { $archive = "golangci-lint-$($env:GOLANGCI_VERSION)-windows-arm64.zip" }
    default { throw "Unsupported golangci-lint platform windows/$goArch" }
}

$tmpArchive = Join-Path $env:TEMP $archive
$tmpDir = Join-Path $env:TEMP ("golangci-lint-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmpDir | Out-Null

Invoke-WebRequest -Uri "https://github.com/golangci/golangci-lint/releases/download/$golangciTag/$archive" -OutFile $tmpArchive

Expand-Archive -Path $tmpArchive -DestinationPath $tmpDir -Force

$folder = "golangci-lint-$($env:GOLANGCI_VERSION)-windows-$goArch"
Copy-Item -Force (Join-Path $tmpDir "$folder\golangci-lint.exe") $env:GOLANGCI_BIN

Remove-Item -Recurse -Force $tmpDir
Remove-Item -Force $tmpArchive
