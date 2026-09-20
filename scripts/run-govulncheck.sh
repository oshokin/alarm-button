#!/usr/bin/env bash

set -euo pipefail

if [[ -z "${GOVULNCHECK_BIN:-}" ]]; then
  echo "GOVULNCHECK_BIN is required" >&2
  exit 2
fi

"${GOVULNCHECK_BIN}" ./...
