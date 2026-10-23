# Bound Codex guard HTTP isolation

The plugin requests transport policy through one additive SDK field:
`HostModelExecutionRequest.HTTPTransport` (`http_transport` in callback JSON).

| Value | Behavior |
| --- | --- |
| empty | Existing native transport policy |
| `session` | Private parent pool; reuse within the same session and bound guard execution |
| `isolated` | New socket for this execution; close at response completion |
| `isolated_http1` | New socket and HTTP/1.1 only; close at response completion |

The field applies only to bound Codex model-guard callbacks. Unbound calls reject
it. An unknown policy fails before model dispatch. The host keeps native proxy,
credential, response-size and plugin-interceptor behavior.

For API-key upstreams, the host clones the selected `http.Transport` into a privately owned
pool. Session mode reuses that pool until session/route change or guard completion.
Isolated modes disable connection reuse. HTTP/1.1 mode restricts both the HTTP transport and TLS ALPN.
It never closes the shared transport pool. An opaque custom transport or custom
TLS dialer is rejected because it cannot promise a new socket and protocol.
That local isolation error is not charged as an upstream model failure.

ChatGPT uTLS already owns a separate socket per request. Side mode changes its
Chrome ClientHello ALPN to HTTP/1.1 and removes HTTP/2 application settings.
It keeps certificate verification and the configured proxy dialer.

The ModelTrace plugin opts every guarded parent/steer request into `session`
and every side request into `isolated_http1`. A side neither borrows nor closes
the parent pool. Before dispatch on a changed session, the host closes all sockets
owned by the old parent pool, including any active socket. Guard completion also
closes its pool. No pool persists beyond this bound host request. Other requests
keep native pooling. No client HTTP header can enable this callback policy.

The standard transport emits `guard_http_connection` and `guard_http_closed`
through infra logging when trace is enabled. `guard_http_response_closed` marks
a finished parent response while its session connection remains reusable. Fields contain protocol, reuse,
socket addresses, and normal session/thread IDs. They contain no auth or state
token. An E2E audit verifies old-session closure before rotation and HTTP/1.1 side selection.

Local HTTPS tests exercise the actual CPA bound Execute/Stream callbacks with
a server that supports HTTP/2. They verify parent HTTP/2, side HTTP/1.1, fresh
sockets after rotation, parent reuse across side calls, and no mutation or closure
of the shared transport. A separate
ClientHello test checks the uTLS side ALPN. Race checks cover these paths.

SDK changes remain in `patches/cliproxyapi-sdk/feat/plugin-contracts`. Host
changes are folded into the existing owner patches. New transport files belong
to `codex-session-context`. The immutable upstream pin plus all patches is
verified against the complete materialized source tree.
