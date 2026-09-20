#!/usr/bin/env bash

set -euo pipefail

golangci_tag="v${GOLANGCI_VERSION}"

if [[ "${GOLANGCI_CURRENT_TAG:-}" == "${golangci_tag}" ]]; then
  exit 0
fi

if [[ -x "${GOLANGCI_BIN:-}" ]]; then
  installed_tag="$("${GOLANGCI_BIN}" version 2>/dev/null | sed -nE 's/.*has version v?([0-9]+\.[0-9]+\.[0-9]+).*/v\1/p')"
  if [[ "${installed_tag}" == "${golangci_tag}" ]]; then
    exit 0
  fi
fi

echo "Installing golangci-lint, version: ${golangci_tag}..."

goos="$(go env GOOS)"
goarch="$(go env GOARCH)"

case "${goos}/${goarch}" in
  linux/amd64) archive="golangci-lint-${GOLANGCI_VERSION}-linux-amd64.tar.gz" ;;
  linux/arm64) archive="golangci-lint-${GOLANGCI_VERSION}-linux-arm64.tar.gz" ;;
  darwin/amd64) archive="golangci-lint-${GOLANGCI_VERSION}-darwin-amd64.tar.gz" ;;
  darwin/arm64) archive="golangci-lint-${GOLANGCI_VERSION}-darwin-arm64.tar.gz" ;;
  *)
    echo "Unsupported golangci-lint platform ${goos}/${goarch}"
    exit 1
    ;;
esac

tmp_archive="/tmp/${archive}"
tmp_dir="$(mktemp -d)"

curl -fsSL "https://github.com/golangci/golangci-lint/releases/download/${golangci_tag}/${archive}" -o "${tmp_archive}"

tar -xzf "${tmp_archive}" -C "${tmp_dir}"
cp "${tmp_dir}/golangci-lint-${GOLANGCI_VERSION}-${goos}-${goarch}/golangci-lint" "${GOLANGCI_BIN}"
chmod +x "${GOLANGCI_BIN}"

rm -rf "${tmp_dir}" "${tmp_archive}"
