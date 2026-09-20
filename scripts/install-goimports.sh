#!/usr/bin/env bash

set -euo pipefail

if [[ "${GOIMPORTS_CURRENT_TAG:-}" == "${GOIMPORTS_TAG:-}" ]]; then
  exit 0
fi

echo "Installing goimports, version: ${GOIMPORTS_TAG}..."
GOBIN="${LOCAL_BIN}" go install golang.org/x/tools/cmd/goimports@"${GOIMPORTS_TAG}"
