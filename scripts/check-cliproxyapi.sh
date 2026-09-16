#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bash "${repo_root}/scripts/materialize-cliproxyapi.sh"

if find "${repo_root}/vendor/cliproxyapi" -name .git -print -quit | grep -q .; then
  printf 'nested .git found in generated CLIProxyAPI checkout\n' >&2
  exit 1
fi

(
  cd "${repo_root}/vendor/cliproxyapi"
  GOFLAGS="${GOFLAGS:--mod=mod}" go test \
    ./cmd/server \
    ./internal/api \
    ./internal/config \
    ./internal/pluginhost \
    ./sdk/cliproxy
)
