$ErrorActionPreference = "Stop"

Write-Host "Installing govulncheck, version: $($env:GOVULNCHECK_TAG)..."
$env:GOBIN = $env:LOCAL_BIN
go install "golang.org/x/vuln/cmd/govulncheck@$($env:GOVULNCHECK_TAG)"
