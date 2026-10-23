#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
bash "${repo_root}/scripts/materialize-cliproxyapi.sh"
infra_go="${INFRA_GO_DIR:-${HOME}/infra/go}"
infra_go="$(cd "${repo_root}" && cd "${infra_go}" && pwd)"

if find "${repo_root}/build/cliproxyapi" -name .git -print -quit | grep -q .; then
  printf 'nested .git found in generated CLIProxyAPI checkout\n' >&2
  exit 1
fi

(
  cd "${repo_root}/build/cliproxyapi"
  check_go_flags="${GOFLAGS:-}"
  export CGO_CFLAGS="${CGO_CFLAGS:-} -I${infra_go}/../c/plugin-interop/include"
  GOFLAGS="${check_go_flags//-mod=mod/}" go build -tags cpa_hold_buffer -o test-output ./cmd/server
  rm test-output
)
