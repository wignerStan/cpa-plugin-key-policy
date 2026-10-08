#!/usr/bin/env python3
"""Verify the pinned, generated infra snapshot and install it into CPA's build."""
import hashlib
import json
from pathlib import Path
import shutil
import sys

root = Path(__file__).resolve().parent.parent
manifest = json.loads((root / "patches/cliproxyapi/manifest.json").read_text())
snapshot = manifest["infra_diagnostics"]
source = root / "third_party/infra"
output = root / "build/cliproxyapi/internal/infra"
if source.is_symlink() or output.is_symlink():
    raise SystemExit("refusing symlinked infra snapshot/output")
expected = set()
for entry in snapshot["files"]:
    relative = Path(entry["path"])
    if relative.is_absolute() or ".." in relative.parts:
        raise SystemExit("invalid infra snapshot path")
    path = source / relative
    if any(parent.is_symlink() for parent in [path, *path.parents]):
        raise SystemExit(f"refusing symlinked infra snapshot file: {relative}")
    if not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != entry["sha256"]:
        raise SystemExit(f"infra snapshot checksum mismatch: {relative}")
    expected.add(relative.as_posix())
if any(path.is_symlink() for path in source.rglob("*")):
    raise SystemExit("refusing any symlink in the infra snapshot")
actual = {path.relative_to(source).as_posix() for path in source.rglob("*") if path.is_file()}
if actual != expected:
    raise SystemExit(f"unmanifested/missing infra snapshot files: {sorted(actual ^ expected)}")
if sys.argv[1:] == ["--check"]:
    print(f"verified {len(expected)} pinned infra diagnostic files")
elif not sys.argv[1:]:
    if any(parent.is_symlink() for parent in output.parents):
        raise SystemExit("refusing symlinked infra output parent")
    if output.exists():
        shutil.rmtree(output)
    shutil.copytree(source / "go/observability", output)
    print(f"installed {len(expected)} pinned infra diagnostic files")
else:
    raise SystemExit("usage: vendor-infra-diagnostics.py [--check]")
