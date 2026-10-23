# Direct infra diagnostics

CPA imports the canonical infra Go modules `logging`,
`observability/diagnosticoutput` and `observability/artifactcapture`. The source owner is
`https://github.com/wignerStan/infra`; no diagnostic source snapshot is retained
in this repository or copied into CPA.

The default module path is the existing checkout at `$HOME/infra/go`.
`INFRA_GO_DIR` can select another infra Go checkout. The expected infra revision
is recorded in `infra-file-notifications.json`; CI checks out that revision at
`build/infra-source` and binds its modules directly.
The materializer writes explicit `go.work` replacements for these focused public
modules and the two cache modules. It does not generate module manifests or
inherit the unrelated infra root dependency graph. These new infra modules are
first committed in infra at `ae25cade115b92bd31fdcc99c8b22ea37cbfd201`.
The current pin also includes cancellable disk transaction admission and shutdown
drain from `7b1fea6d`. Watcher health remains available during a long transaction.
The local replacement selects that checkout; publication and module versions remain
a separate release step.

CPA selects visible protocol fields and trace/dump enablement. Infra owns
credential classification, bounded diagnostic serialization, capture queues,
private artifact files and retention. Ordinary cookies, including `session`,
`sid`, JWT-shaped routing cookies, `__oailb` and `__cflb`, remain readable.
Only explicit authentication names are hashed. CPA additionally identifies
`__Secure-session` as an authentication cookie. LB headers, IDs, turn state and
cache statistics remain readable.

Trace uses infra `DiagnosticJSONSink`; logrus is the compatibility output writer
and trace-level switch. Raw dumps remain separately enabled and use infra
`artifactcapture`. Raw dump bytes are exact and therefore are not sanitized.
Stream deltas do not write trace records. Both trace records and capture queues
have bounds; invalid/oversized trace JSON emits no partial detail.
