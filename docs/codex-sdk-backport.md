# Standalone Codex SDK backport

This branch adds the independent `sdk/protocol` module to the published CPA
patch series. It preserves the host's selected upstream commit. The SDK uses
only the Go standard library and does not load the host or plugin runtime.

The SDK source is an explicit backport of owner commits
`3e4c0ee5300575bf3ca2b82bde27f5577416202e` (the standalone contract) and
`b008c3acec8844c921834d014543605b64da1f32` (message-local decoding state).
Those local commits contain unpublished ancestor history that GitHub rejects.
This branch starts from published commit
`9199e54aa08eb31f2a64c99a4d6908f8df43c1ce` and imports the SDK patches only.
It does not merge that unpublished history.

The SDK bundle bytes match the reviewed bundle exactly. Its `apply_order.txt`
records each patch digest. The host manifest and source relationship also list
the same ordered SDK patches and the resulting complete source tree.

`Response` and `Event` replace their inspection values for each JSON message.
Their duplicate-field markers live within one envelope. Reusing a decoder
destination therefore cannot retain earlier usage, headers, or error markers.
Duplicate usage or headers within one message still produce deferred evidence
errors. Upstream failures retain their normal priority.

The parent commit pins the pristine upstream at `third_party/cliproxyapi`.
Initialize it with `git submodule update --init --depth 1 -- third_party/cliproxyapi`.
The materializer checks that committed gitlink, the manifest commit, and the
clean submodule checkout agree. It applies the ordered patches in a disposable
build checkout. The patched output remains at `build/cliproxyapi`.

Run `bash scripts/check-cliproxyapi.sh` to materialize the selected source and
test both the host and the standalone SDK. SDK tests include the shared usage
vectors, duplicate evidence, reused messages, metadata, and tool history.

## Codec ownership and nested accounting

The follow-up patch keeps decoded cache constraints in `sdk/protocol/codex`.
JSON inspection types, envelope decoding, tool history, and metadata rewrites
move together to `sdk/protocol/codexwire`. Import `Response`, `Event`, `Usage`,
and the wire operations from `codexwire`. The former wire paths have no aliases.
Both packages remain in the existing standalone module.

Decoded validation now carries possible write-count bounds through nested rows.
A known aggregate must fit those bounds even when an intermediate row omits its
write count. A complete child partition reporting two writes rejects aggregate
claims of zero or three. A partial partition can retain a wider valid range.
Validation does not fill missing counters or change the caller's evidence.

The original public SDK decoder accepted the contradictory aggregate claims.
The new wire tests and shared usage vectors cover these cases, deeper unknown
rows, valid partial ranges, and missing write counts. The wire decoder retains
the existing duplicate-field checks and message-local reset behavior.
