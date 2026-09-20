#!/usr/bin/env bash

set -euo pipefail

if [[ "${PROTOC_GEN_GO_GRPC_CURRENT_TAG:-}" == "${PROTOC_GEN_GO_GRPC_TAG:-}" ]]; then
  exit 0
fi

echo "Installing protoc-gen-go-grpc, version: ${PROTOC_GEN_GO_GRPC_TAG}..."
GOBIN="${LOCAL_BIN}" go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@"${PROTOC_GEN_GO_GRPC_TAG}"
