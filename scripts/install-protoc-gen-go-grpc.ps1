$ErrorActionPreference = "Stop"

if ($env:PROTOC_GEN_GO_GRPC_CURRENT_TAG -eq $env:PROTOC_GEN_GO_GRPC_TAG) {
    exit 0
}

Write-Host "Installing protoc-gen-go-grpc, version: $($env:PROTOC_GEN_GO_GRPC_TAG)..."
$env:GOBIN = $env:LOCAL_BIN
go install "google.golang.org/grpc/cmd/protoc-gen-go-grpc@$($env:PROTOC_GEN_GO_GRPC_TAG)"
