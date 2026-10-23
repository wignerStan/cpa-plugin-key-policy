# CLIProxyAPI patches

CLIProxyAPI remains an upstream product. `cliproxyapi/manifest.json` records the
official commit and source tree. This repository has no CPA source subtree or
submodule. The materializer fetches the exact commit into the ignored
`build/upstream/cliproxyapi.git` cache and builds under `build/cliproxyapi`.

- `cliproxyapi-sdk/`: public plugin API/ABI contracts and the standalone `sdk/protocol` Go module.
- `cliproxyapi/`: diagnostics, Codex context, native admission and host wiring.

Each semantic bundle lives at `<family>/feat/<intent>`. It contains one patch
per target file, named by replacing `/` in the target path with `__` and adding
`.patch`. `apply_order.txt` owns the order and SHA-256 hashes; `run.py` owns its
checks. The v4 source relationship orders the SDK and host bundles together.
Each source file belongs to only one bundle. Fold fixes into its existing patch
and regenerate the final delta against the declared upstream. Do not add a later
patch that repairs an earlier patch. Shared assembly files have one owner in
host wiring. All bundles are applied once and validated as one build.

`python3 scripts/check-patch-inventory.py` checks file ownership, checksums and
source-record agreement. Materialization verifies the pinned upstream tree
before applying patches. The first build fetches the exact commit; later builds
reuse the local object cache. No source submodule is required.
`bash scripts/check-cliproxyapi.sh` applies to the clean pin, verifies the resulting
tree, runs focused tests and builds the server.
The source relationship is
`docs/source-relationships/cliproxyapi-materialization.json`. Its upstream URL
and commit are the source authority. The relationship records the ordered
bundles and final tree; the manifest also pins the original source tree.
Infra is a direct module/header dependency, not a patch overlay or copied source.
