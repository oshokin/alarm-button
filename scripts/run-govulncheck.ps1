$ErrorActionPreference = "Stop"

if ([string]::IsNullOrWhiteSpace($env:GOVULNCHECK_BIN)) {
    throw "GOVULNCHECK_BIN is required"
}

& $env:GOVULNCHECK_BIN ./...
exit $LASTEXITCODE
