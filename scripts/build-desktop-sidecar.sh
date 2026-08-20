#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
host_triple="$(rustc -vV 2>/dev/null | sed -n 's/^host: //p')"

if [[ -z "${host_triple}" ]]; then
  echo "rustc not found; cannot build desktop sidecar" >&2
  exit 1
fi

output="${repo_dir}/desktop/src-tauri/binaries/omnillm-${host_triple}"
mkdir -p "$(dirname "${output}")"

echo "Building ${output}"
go build -o "${output}" "${repo_dir}"
