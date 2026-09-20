#!/usr/bin/env bash

set -euo pipefail

if [[ "$#" -lt 4 ]]; then
  echo "usage: $0 <local_bin> <cmd_dir> <taskfile_dir> <binary...>"
  exit 1
fi

local_bin="$1"
cmd_dir="$2"
taskfile_dir="$3"
shift 3
binaries=("$@")

mkdir -p "${local_bin}"

git_tag="$(git -C "${taskfile_dir}" describe --tags --abbrev=0 2>/dev/null || true)"
version="${git_tag#v}"
if [[ -z "${version}" ]]; then
  version="dev"
fi

for binary in "${binaries[@]}"; do
  package_path="${cmd_dir}/${binary}"
  echo "Building ${binary} (version=${version})..."
  go build \
    -trimpath \
    -ldflags "-X github.com/oshokin/alarm-button/internal/version.Version=${version}" \
    -o "${local_bin}/${binary}" \
    "${package_path}"
done
