$ErrorActionPreference = "Stop"

param(
    [Parameter(Mandatory = $true)][string]$LocalBin,
    [Parameter(Mandatory = $true)][string]$CmdDir,
    [Parameter(Mandatory = $true)][string]$TaskfileDir,
    [Parameter(Mandatory = $true)][string[]]$Binaries
)

New-Item -ItemType Directory -Force -Path $LocalBin | Out-Null

$gitTag = ""
try {
    $gitTag = (git -C $TaskfileDir describe --tags --abbrev=0 2>$null).Trim()
} catch {
    $gitTag = ""
}

$version = $gitTag -replace "^v", ""
if ([string]::IsNullOrWhiteSpace($version)) {
    $version = "dev"
}

foreach ($binary in $Binaries) {
    $packagePath = Join-Path $CmdDir $binary
    $outputPath = Join-Path $LocalBin "$binary.exe"
    Write-Host "Building $binary (version=$version)..."
    go build `
        -trimpath `
        -ldflags "-X github.com/oshokin/alarm-button/internal/version.Version=$version" `
        -o $outputPath `
        $packagePath
}
