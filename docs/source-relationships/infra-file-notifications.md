# Infra file configuration

The plugin imports `github.com/wignerStan/infra/go/config/file` directly. Its
implementation belongs to `https://github.com/wignerStan/infra`; CPA contains no
copy. The focused module depends on the canonical `files/notify` adapter and
avoids the infra root module's unrelated dependencies. Both replacements are
bound by `scripts/prepare-go-workspace.sh`.

Infra owns OS subscription lifetime, effective-revision deduplication, pending
normalization detection, reload health callbacks, and private atomic file writes.
A pending rewrite always runs even when a plaintext key hashes to the existing
credential. Rewritten documents use mode 0600. Neither the watcher nor its
fingerprint retains or logs document bytes. There is no polling loop.

CPA owns JSON decoding, key hashing, model permissions, catalog validation, and
cache publication. It excludes usage and timestamps from effective revisions.
Policy reload holds the policy writer lock so an old snapshot cannot overwrite a
later management commit. Invalid documents preserve the last valid policy.

For local builds set `INFRA_GO_DIR` to the canonical checkout's `go` directory;
it defaults to `$HOME/infra/go`. The helper writes an ignored `go.work` with
replacements for `config/file` and `files/notify`. Existing user-managed
workspaces are left intact. The relative replacements in `go.mod` also support
the normal sibling-checkout layout.

```sh
make test INFRA_GO_DIR=/path/to/infra/go
```

CI and Homebrew stage the infra revision recorded in
`infra-file-notifications.json`. Publish that revision before updating the pin.
The underlying OS subscription implementation originates at infra commit
`c1be68e8f818b649e4b66411db76cb8819f6f79b`.
