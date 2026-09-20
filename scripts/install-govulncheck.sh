#!/usr/bin/env bash

set -euo pipefail

echo "Installing govulncheck, version: ${GOVULNCHECK_TAG}..."
GOBIN="${LOCAL_BIN}" go install golang.org/x/vuln/cmd/govulncheck@"${GOVULNCHECK_TAG}"
