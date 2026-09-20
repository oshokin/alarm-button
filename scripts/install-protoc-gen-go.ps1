$ErrorActionPreference = "Stop"

if ($env:PROTOC_GEN_GO_CURRENT_TAG -eq $env:PROTOC_GEN_GO_TAG) {
    exit 0
}

Write-Host "Installing protoc-gen-go, version: $($env:PROTOC_GEN_GO_TAG)..."
$env:GOBIN = $env:LOCAL_BIN
go install "google.golang.org/protobuf/cmd/protoc-gen-go@$($env:PROTOC_GEN_GO_TAG)"
