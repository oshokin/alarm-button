#!/usr/bin/env bash

set -euo pipefail

if [[ "${PROTOC_GEN_GO_CURRENT_TAG:-}" == "${PROTOC_GEN_GO_TAG:-}" ]]; then
  exit 0
fi

echo "Installing protoc-gen-go, version: ${PROTOC_GEN_GO_TAG}..."
GOBIN="${LOCAL_BIN}" go install google.golang.org/protobuf/cmd/protoc-gen-go@"${PROTOC_GEN_GO_TAG}"
