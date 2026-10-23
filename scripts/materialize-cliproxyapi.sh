#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"
manifest="${repo_root}/patches/cliproxyapi/manifest.json"
output="${CLIPROXYAPI_BUILD_DIR:-${repo_root}/build/cliproxyapi}"

if [[ ! -f "${manifest}" ]]; then
  printf 'missing CLIProxyAPI manifest: %s\n' "${manifest}" >&2
  exit 1
fi

case "${output}" in
  "${repo_root}/build/cliproxyapi") ;;
  *)
    printf 'refusing output outside build/cliproxyapi: %s\n' "${output}" >&2
    exit 1
    ;;
esac

build_root="${repo_root}/build"
if [[ -L "${build_root}" || ( -e "${build_root}" && ! -d "${build_root}" ) ]]; then
  printf 'refusing foreign build root: %s\n' "${build_root}" >&2
  exit 1
fi
mkdir -p "${build_root}"
lock_dir="${build_root}/.cliproxyapi-materialization.lock"
if ! mkdir "${lock_dir}" 2>/dev/null; then
  printf 'another CLIProxyAPI materialization holds the build lock: %s\n' "${lock_dir}" >&2
  exit 1
fi
control_dir=""
recovery_dir=""
cleanup() {
  local status=$?
  trap - EXIT
  if [[ "${status}" != 0 && -n "${recovery_dir}" && ( -e "${recovery_dir}/source" || -L "${recovery_dir}/source" ) ]]; then
    if [[ ! -e "${output}" && ! -L "${output}" ]]; then
      if mv "${recovery_dir}/source" "${output}"; then
        printf 'restored previous CLIProxyAPI build: %s\n' "${output}" >&2
      else
        printf 'previous CLIProxyAPI build retained at: %s/source\n' "${recovery_dir}" >&2
      fi
    else
      printf 'previous CLIProxyAPI build retained at: %s/source\n' "${recovery_dir}" >&2
    fi
  fi
  if [[ -n "${control_dir}" ]]; then
    rm -rf "${control_dir}"
  fi
  rm -f "${lock_dir}/owner"
  rmdir "${lock_dir}"
  exit "${status}"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
printf 'pid=%s\n' "$$" > "${lock_dir}/owner"

manifest_value() {
  python3 - "${manifest}" "$1" <<'PY'
import json
import sys

manifest_path, expression = sys.argv[1:]
with open(manifest_path, encoding="utf-8") as handle:
    value = json.load(handle)

for part in expression.split('.'):
    value = value[part]
print(value)
PY
}

# Direct module dependency: use the selected infra checkout without copying it.
infra_go="${INFRA_GO_DIR:-${HOME}/infra/go}"
infra_go="$(cd "${infra_go}" && pwd)"
for adapter in cache/disk cache/writeback logging observability/diagnosticoutput observability/artifactcapture; do
  if [[ ! -f "${infra_go}/${adapter}/go.mod" ]]; then
    printf 'missing direct infra module: %s (set INFRA_GO_DIR)\n' "${infra_go}/${adapter}" >&2
    exit 1
  fi
done


source_commit="$(manifest_value upstream.commit)"
source_tree="$(manifest_value upstream.tree)"
source_url="$(manifest_value upstream.url)"
# One source file has one owning feature patch; no patch-over-patch repairs.
patch_inventory="$(python3 "${repo_root}/scripts/check-patch-inventory.py" --list-patches)"

# The product stays upstream. Cache only the exact required Git object graph
# under ignored build/, then export the pinned tree for patch application.
source_cache="${repo_root}/build/upstream/cliproxyapi.git"
if [[ -L "${source_cache}" ]]; then
  printf 'refusing source cache symlink: %s\n' "${source_cache}" >&2
  exit 1
fi
if [[ ! -e "${source_cache}" ]]; then
  git init --bare -q --object-format=sha1 "${source_cache}"
fi
if [[ "$(git --git-dir="${source_cache}" rev-parse --is-bare-repository)" != true ]]; then
  printf 'source cache is not a bare Git repository: %s\n' "${source_cache}" >&2
  exit 1
fi
if ! git --git-dir="${source_cache}" cat-file -e "${source_commit}^{commit}" 2>/dev/null; then
  git --git-dir="${source_cache}" fetch --no-tags --depth=1 "${source_url}" "${source_commit}"
fi
actual_source_tree="$(git --git-dir="${source_cache}" rev-parse "${source_commit}^{tree}")"
if [[ "${actual_source_tree}" != "${source_tree}" ]]; then
  printf 'upstream tree differs from manifest: %s != %s\n' "${actual_source_tree}" "${source_tree}" >&2
  exit 1
fi

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# Staging and publication share a filesystem; the previous build stays intact
# until every patch and bundle runner has passed.
control_dir="$(mktemp -d "${build_root}/.cliproxyapi-stage.XXXXXX")"
work_dir="${control_dir}/source"
mkdir "${work_dir}"

verify_tree() {
  local candidate="$1" expected="$2" snapshot="${control_dir}/snapshot"
  if [[ ! -d "${candidate}" || -L "${candidate}" || -n "$(find "${candidate}" -name .git -print -quit)" ]]; then
    printf 'refusing to replace foreign build path: %s\n' "${candidate}" >&2
    return 1
  fi
  rm -rf "${snapshot}"
  mkdir "${snapshot}"
  rsync -a --exclude=/go.work --exclude=/go.work.sum "${candidate}/" "${snapshot}/"
  if [[ -n "$(find "${snapshot}" -name .git -print -quit)" ]]; then
    printf 'foreign Git entry appeared while checking build: %s\n' "${candidate}" >&2
    return 1
  fi
  git init -q --object-format=sha1 "${snapshot}"
  git -C "${snapshot}" -c core.autocrlf=false -c core.filemode=true -c core.attributesfile=/dev/null add --force --all
  if [[ "$(git -C "${snapshot}" write-tree)" != "${expected}" ]]; then
    printf 'CLIProxyAPI build has local changes; preserving %s\n' "${candidate}" >&2
    return 1
  fi
  rm -rf "${snapshot}"
}

# Generated builds may have been edited for patch development. Never erase
# such changes, or a foreign Git checkout, when rematerializing.
receipt="${repo_root}/build/cliproxyapi.receipt.json"
had_output=false
if [[ -e "${output}" || -L "${output}" ]]; then
  had_output=true
  previous_tree="$(python3 - "${receipt}" "${manifest}" <<'PY'
from pathlib import Path
import json
import re
import sys
receipt, manifest = map(Path, sys.argv[1:])
tree = json.loads((receipt if receipt.exists() else manifest).read_text())["result_tree"]
if not isinstance(tree, str) or not re.fullmatch(r"[0-9a-f]{40}", tree):
    raise SystemExit("invalid previous build tree identity")
print(tree)
PY
)"
  verify_tree "${output}" "${previous_tree}"
fi

git --git-dir="${source_cache}" archive "${source_commit}" | tar -x -C "${work_dir}"
git init -q --object-format=sha1 "${work_dir}"
git -C "${work_dir}" config user.name "cpa-key-policy materializer"
git -C "${work_dir}" config user.email "cpa-key-policy@localhost"
git -C "${work_dir}" config core.autocrlf false
git -C "${work_dir}" config core.filemode true
git -C "${work_dir}" config core.attributesfile /dev/null
git -C "${work_dir}" config core.hooksPath /dev/null
git -C "${work_dir}" config commit.gpgsign false
git -C "${work_dir}" add --force --all

actual_tree="$(git -C "${work_dir}" write-tree)"
if [[ "${actual_tree}" != "${source_tree}" ]]; then
  printf 'export differs from the immutable upstream tree: %s != %s\n' "${actual_tree}" "${source_tree}" >&2
  exit 1
fi
git -C "${work_dir}" commit -q -m "Build from pristine CLIProxyAPI ${source_commit}"

patch_count=0
while IFS=$'\t' read -r patch_path expected_sha; do
  [[ -n "${patch_path}" ]] || continue
  case "${patch_path}" in
    patches/cliproxyapi/*.patch|patches/cliproxyapi-sdk/*.patch) ;;
    *)
      printf 'refusing patch outside declared CPA/SDK patch roots: %s\n' "${patch_path}" >&2
      exit 1
      ;;
  esac

  patch_file="${repo_root}/${patch_path}"
  if [[ ! -f "${patch_file}" ]]; then
    printf 'missing patch file: %s\n' "${patch_file}" >&2
    exit 1
  fi
  actual_sha="$(sha256_file "${patch_file}")"
  if [[ "${actual_sha}" != "${expected_sha}" ]]; then
    printf 'patch checksum mismatch for %s: %s != %s\n' "${patch_path}" "${actual_sha}" "${expected_sha}" >&2
    exit 1
  fi
  git -C "${work_dir}" apply --check "${patch_file}"
  # Bundles contain both mail patches and plain unified diffs. The resulting
  # source tree is authoritative; Git mail-envelope detection is not required.
  git -C "${work_dir}" apply --index "${patch_file}"
  git -C "${work_dir}" commit -q -m "Apply ${patch_path}"
  patch_count=$((patch_count + 1))
done <<< "${patch_inventory}"

result_tree="$(git -C "${work_dir}" write-tree)"
if [[ "${result_tree}" != "$(manifest_value result_tree)" ]]; then
  printf 'materialized tree differs from the manifest: %s\n' "${result_tree}" >&2
  exit 1
fi

python3 - "${control_dir}/receipt.json" "${source_commit}" "${result_tree}" <<'PY'
from pathlib import Path
import json
import sys
path, commit, tree = sys.argv[1:]
Path(path).write_text(json.dumps({"source_commit": commit, "result_tree": tree}) + "\n")
PY

# go.work is a local build binding, not part of the upstream source patch/tree.
# The workspace replace keeps this unreleased focused adapter local and avoids
# pulling unrelated dependencies from the infra root module.
python3 - "${work_dir}" "${infra_go}" <<'PYWORK'
from pathlib import Path
import json
import sys
output, infra = [Path(p).resolve() for p in sys.argv[1:]]
(output / "go.work").write_text(
    'go 1.26.6\n\nuse .\n\n' + ''.join(
        'replace github.com/wignerStan/infra/go/' + name + ' => '
        + json.dumps(str(infra / name)) + '\n'
        for name in ('cache/disk', 'cache/writeback', 'logging', 'observability/diagnosticoutput', 'observability/artifactcapture')))

PYWORK

# Each semantic bundle owns its test boundary. All bundles are applied before
# any runner starts, so SDK contracts and host implementations compile together.
bundle_inventory="$(python3 "${repo_root}/scripts/check-patch-inventory.py" --list-bundles)"
while IFS= read -r bundle; do
  (
    cd "${work_dir}"
    check_go_flags="${GOFLAGS:-}"
    export GOFLAGS="${check_go_flags//-mod=mod/-mod=readonly}"
    export CGO_CFLAGS="${CGO_CFLAGS:-} -I${infra_go}/../c/plugin-interop/include"
    export GOWORK="${work_dir}/go.work"
    export DOTAGENTS_REPO_ROOT="${repo_root}"
    export DOTAGENTS_SOURCE_PATH="${work_dir}"
    export DOTAGENTS_BUILD_PATH="${work_dir}"
    export DOTAGENTS_PATCH_BUNDLE="${repo_root}/${bundle}"
    python3 "${repo_root}/${bundle}/run.py"
  )
done <<< "${bundle_inventory}"

# Runners may write files. Recheck the complete staged source before publishing.
rm -rf "${work_dir}/.git"
verify_tree "${work_dir}" "${result_tree}"
if [[ "${had_output}" == true ]]; then
  verify_tree "${output}" "${previous_tree}"
  recovery_dir="$(mktemp -d "${build_root}/cliproxyapi-recovery.XXXXXX")"
  printf 'recovery_snapshot=%s\n' "${recovery_dir}"
  if [[ -f "${receipt}" ]]; then
    cp "${receipt}" "${recovery_dir}/receipt.json"
  fi
  mv "${output}" "${recovery_dir}/source"
  # Capture edits or a foreign checkout inserted between the last check and
  # rename. On failure cleanup restores it, or leaves the recovery path intact.
  verify_tree "${recovery_dir}/source" "${previous_tree}"
elif [[ -e "${output}" || -L "${output}" ]]; then
  printf 'build path appeared during materialization; preserving %s\n' "${output}" >&2
  exit 1
fi

# Never replace a path created by another actor after moving the old build.
python3 - "${work_dir}" "${output}" <<'PYPUBLISH'
import os
import sys
source, destination = sys.argv[1:]
if os.path.lexists(destination):
    raise SystemExit('build path appeared during publication; preserving it')
os.rename(source, destination)
PYPUBLISH
mv "${control_dir}/receipt.json" "${receipt}"
# Keep the previous source directory even on success. An editor can still hold
# an open file in it after rename; this recovery copy must not be deleted here.

printf 'source_commit=%s\nsource_tree=%s\npatch_count=%s\nmaterialized_tree=%s\noutput=%s\n' \
  "${source_commit}" "${source_tree}" "${patch_count}" "${result_tree}" "${output}"
