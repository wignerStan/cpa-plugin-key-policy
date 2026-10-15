# Codex TRACE metadata

TRACE records readable protocol metadata and the decisions needed to debug the
proxy. Response summaries, turn state, session IDs, response IDs, routing values,
usage counters and generic `token` fields are not credentials. Their selected
values stay readable. A JWT-shaped turn-state string is still turn state; shape
alone does not make arbitrary metadata an authentication credential.

Infra owns the shared `diagnosticoutput.Projection` primitive. It copies selected
JSON fields and applies credential classification in one traversal. CPA supplies
the protocol field names, permitted metadata containers and event boundaries.
It does not duplicate the traversal or credential hashing implementation.

Actual API/auth fields and configured credential aliases are hashed. Authorization
retains its scheme. Known authentication cookies are hashed. Generic session IDs, load-balancer
cookies and cookie attributes stay readable, even when values look like JWTs.
Cookie JWT-shape detection is opt-in. Explicit `at` and `rt` credentials remain
hashed in fields and cookie values. Headers and payload metadata use the same
infra policy; there is no separate application cookie parser.
SHA-256 comparison values in investigation reports are optional integrity
identifiers, not a reason to hide nonsecret TRACE fields.

Provider response summaries are visible at complete summary or response events.
This includes summary text done, summary part done, reasoning item done and
reasoning summaries in completed responses. Only selected summary fields are
included. A client request cannot gain summary visibility by using a response
event type. Request prompts, ordinary answer text, tool arguments and encrypted
content do not become metadata through this change.

WebSocket TRACE uses the provider directions `upstream_in` and `client_out` for
summary visibility. HTTPS/SSE TRACE uses the existing response framer, after a
complete event is ready to write. Delta events do not emit TRACE records. The
projection is bounded by the existing 8 KiB view limit; larger views report the
limit instead of printing an arbitrary prefix. Trace verbosity remains a separate
setting from raw dump consent and metadata-only access logging.

Raw dump chunks retain the existing infra buffered capture path. The new SSE
metadata trace does not recapture raw bytes, add a stream parser, change wire
bytes or add per-delta disk writes. Raw capture remains a separately enabled,
bounded, best effort diagnostic artifact.
