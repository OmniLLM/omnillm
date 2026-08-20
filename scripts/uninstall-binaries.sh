#!/usr/bin/env bash
set -euo pipefail

install_dir="$(go env GOBIN)"
if [[ -z "${install_dir}" ]]; then
  go_path="$(go env GOPATH)"
  install_dir="${go_path%%:*}/bin"
fi

for name in omnillm omniproxy; do
  path="${install_dir}/${name}"
  if [[ -f "${path}" ]]; then
    rm -- "${path}"
    echo "Removed ${path}"
  fi
done
