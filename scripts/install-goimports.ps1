$ErrorActionPreference = "Stop"

if ($env:GOIMPORTS_CURRENT_TAG -eq $env:GOIMPORTS_TAG) {
    exit 0
}

Write-Host "Installing goimports, version: $($env:GOIMPORTS_TAG)..."
$env:GOBIN = $env:LOCAL_BIN
go install "golang.org/x/tools/cmd/goimports@$($env:GOIMPORTS_TAG)"
