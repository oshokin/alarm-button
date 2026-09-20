#!/usr/bin/env bash

set -euo pipefail

echo "Generating protobuf code..."
mkdir -p internal/pb

PATH="${LOCAL_BIN}:${PATH}" "${PROTOC_BIN}" \
  --go_out=internal/pb \
  --go_opt=paths=source_relative \
  --go-grpc_out=internal/pb \
  --go-grpc_opt=paths=source_relative \
  --proto_path="${LOCAL_BIN}/include" \
  --proto_path=api \
  api/v1/*.proto

echo "Formatting generated code..."
"${GOIMPORTS_BIN}" -w ./
echo "Protobuf generation completed successfully!"
