#!/usr/bin/env bash

set -euo pipefail

if [[ "${PROTOC_CURRENT_TAG:-}" == "${PROTOC_VERSION:-}" ]]; then
  exit 0
fi

echo "Installing protoc, version: ${PROTOC_VERSION}..."

curl -fsSL -o "${PROTOC_ZIP_FULL_PATH}" "${PROTOC_URL}"

unzip -o "${PROTOC_ZIP_FULL_PATH}" -d "${LOCAL_BIN}"
chmod +x "${PROTOC_BIN_IN_ZIP}"

rm -f "${PROTOC_ZIP_FULL_PATH}"
mv "${PROTOC_BIN_IN_ZIP}" "${PROTOC_BIN}"
rm -rf "${LOCAL_BIN:?}/bin" "${LOCAL_BIN:?}/readme.txt"
