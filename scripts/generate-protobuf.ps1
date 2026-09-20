$ErrorActionPreference = "Stop"

Write-Host "Generating protobuf code..."
New-Item -ItemType Directory -Force -Path "internal/pb" | Out-Null

$env:PATH = "$($env:LOCAL_BIN);$($env:PATH)"

& $env:PROTOC_BIN `
    --go_out=internal/pb `
    --go_opt=paths=source_relative `
    --go-grpc_out=internal/pb `
    --go-grpc_opt=paths=source_relative `
    --proto_path="$($env:LOCAL_BIN)\include" `
    --proto_path=api `
    api/v1/*.proto

Write-Host "Formatting generated code..."
& $env:GOIMPORTS_BIN -w ./
Write-Host "Protobuf generation completed successfully!"
