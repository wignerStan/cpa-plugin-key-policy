# Infra diagnostic source snapshot

`third_party/infra` contains generated, byte-identical source from the pinned
infra checkout recorded in `patches/cliproxyapi/manifest.json`. The selected
packages use only Go's standard library. Update these files in infra, run its
package tests, then refresh the source snapshot and per-file SHA-256 entries.
Do not maintain another implementation here.

The original root `go.mod` and both licenses are retained as provenance. The CPA
materializer verifies every file and rejects symlinks or unlisted files before
copying the selected package source to `build/cliproxyapi/internal/infra`.
CPA imports that internal source path explicitly. It does not impersonate the
upstream module, generate a module, or change CPA's unrelated dependency graph.

Patch 0012 retains only CPA-specific metadata projection, protocol event selection,
header visibility choices, artifact naming and capture consent. Infra owns credential
classification/hashing, bounded stream storage, queue admission, idle expiration,
shutdown draining, private file writing and retention. CPA selects all regular
`.raw` files in its dedicated dump directory for the retention budget, including
legacy artifact names; symlinks and foreign extensions stay outside that ownership.

Raw capture requires explicit `codex-dump` consent and stays independent of trace
verbosity. It is best effort diagnostic evidence, not durable business data.
Streaming chunks do not emit trace records, perform disk I/O or acquire shared
capture mutexes. A known queue or byte-budget loss discards pending capture and
withdraws the owner until reconfiguration; it must not publish a partial artifact
as a complete capture. Configuration changes and shutdown may wait for drain.
