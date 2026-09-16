#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
manifest="${repo_root}/patches/cliproxyapi/manifest.json"
output="${CLIPROXYAPI_VENDOR_DIR:-${repo_root}/vendor/cliproxyapi}"

if [[ ! -f "${manifest}" ]]; then
  printf 'missing CLIProxyAPI manifest: %s\n' "${manifest}" >&2
  exit 1
fi

case "${output}" in
  "${repo_root}/vendor/cliproxyapi") ;;
  *)
    printf 'refusing output outside vendor/cliproxyapi: %s\n' "${output}" >&2
    exit 1
    ;;
esac

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

source_url="${CLIPROXYAPI_SOURCE_URL:-$(manifest_value upstream.url)}"
source_commit="${CLIPROXYAPI_SOURCE_COMMIT:-$(manifest_value upstream.commit)}"

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

work_dir="$(mktemp -d "${TMPDIR:-/tmp}/cpa-key-policy.XXXXXX")"
trap 'rm -rf "${work_dir}"' EXIT

git init -q "${work_dir}"
git -C "${work_dir}" config user.name "cpa-key-policy materializer"
git -C "${work_dir}" config user.email "cpa-key-policy@localhost"
git -C "${work_dir}" remote add upstream "${source_url}"
git -C "${work_dir}" fetch --filter=blob:none --no-tags --depth=1 upstream "${source_commit}"
git -C "${work_dir}" checkout -q --detach FETCH_HEAD

actual_commit="$(git -C "${work_dir}" rev-parse HEAD)"
if [[ "${actual_commit}" != "${source_commit}" ]]; then
  printf 'fetched commit %s, expected %s\n' "${actual_commit}" "${source_commit}" >&2
  exit 1
fi

patch_count=0
while IFS=$'\t' read -r patch_path expected_sha; do
  [[ -n "${patch_path}" ]] || continue
  case "${patch_path}" in
    patches/cliproxyapi/*.patch) ;;
    *)
      printf 'refusing patch outside patches/cliproxyapi: %s\n' "${patch_path}" >&2
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
  git -C "${work_dir}" am --3way --keep-cr "${patch_file}"
  patch_count=$((patch_count + 1))
done < <(python3 - "${manifest}" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    manifest = json.load(handle)

for entry in sorted(manifest["patches"], key=lambda item: item["order"]):
    print(f'{entry["path"]}\t{entry["sha256"]}')
PY
)

if [[ -e "${output}" && -L "${output}" ]]; then
  printf 'refusing to replace symlink: %s\n' "${output}" >&2
  exit 1
fi
mkdir -p "${repo_root}/vendor"
rm -rf "${output}"
mkdir -p "${output}"
rsync -a --delete --exclude='.git' "${work_dir}/" "${output}/"

if find "${output}" -type d -name .git -print -quit | grep -q .; then
  printf 'generated vendor checkout contains a nested .git directory\n' >&2
  exit 1
fi

git -C "${work_dir}" add -A
result_tree="$(git -C "${work_dir}" write-tree)"
printf 'source_commit=%s\npatch_count=%s\nmaterialized_tree=%s\noutput=%s\n' \
  "${source_commit}" "${patch_count}" "${result_tree}" "${output}"
