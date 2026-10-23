# Codex 0.162 interruption compatibility

Codex 0.162 enables immediate interruption by default. A proxy that rejects
`response.interrupt` can therefore fail when new input steers an active turn.
The default change is specific to 0.162; the wire message already existed in
0.161. CLIProxyAPI v8.0.22 includes the official interrupt implementation.
The host now pins v8.0.33 commit `2422c6b0f5fe16495320fa29456493874c1977a0`.
The former local interrupt patch is retired in favor of the upstream fix.

## Verified client history

| Version | Official commit | `features.instant_interrupt` default |
| --- | --- | --- |
| 0.161.0 | `979011409de0a60b52f179721948e65531d26144` | false |
| 0.162.0 | `c1382380de69521303b416720a52f42d51af6248` | true |

The [wire implementation](https://github.com/openai/codex/commit/12de0e395d3313bc564190d983cb4f5acf0be713)
added interruption to preserve WebSocket continuations. The
[default change](https://github.com/openai/codex/commit/406b0c44460cf71e90c21c7abe6e581add3a6421)
lets new input preempt an active response. The feature remains marked under
development in that release. It requires model `use_responses_lite` and an active
WebSocket response with an available response ID. UI steering does not imply a
`response.steer` wire message.

The client sends:

```json
{"type":"response.interrupt","response_id":"resp_1","mode":"discard_partial_items"}
```

It drains the upstream terminal response. An incomplete response whose reason is
`interrupted` ends this generation without ending the turn. The client then sends
its own next create, using `previous_response_id` to retain the continuation.
The inspected release has no interrupt capability negotiation or dedicated
recovery for an unsupported-interrupt 400. This is a compatibility gap exposed
by the default rollout; it does not establish a fault in the client envelope.

## Official upstream interrupt route

The [official interrupt fix](https://github.com/router-for-me/CLIProxyAPI/commit/6331b38)
uses a concurrent downstream reader. It handles `response.interrupt` independently
of the response-steering flag. Native Codex WebSocket interruption therefore
requires an active native upstream socket and an eligible credential, with no
extra response-steering setting. The frame keeps its response ID, mode, and
extension fields. The upstream supplies the interrupted terminal boundary.

The interrupt writer checks that the bound credential remains enabled. It rejects
idle sockets and malformed response IDs without opening a connection. The client
then sends its own continuation on the same socket. The host also includes the
official local cancellation path for in-flight HTTP upstream responses and the
Home execution-session credential lookup. These are upstream behaviors.

Response steering remains a separate opt-in feature. Its canonical v8.0.22
configuration is:

```yaml
upstream:
  codex:
    response-steering: true
```

Historical `oauth.providers.codex.response-steering` and
`codex.response-steering` spellings migrate to that canonical path. The upgrade
leaves configuration defaults unchanged.

## Local patches and diagnostics

The consolidated infra diagnostics patch preserves private,
bounded diagnostic capture through the new reader and interrupt writer. It
captures the original downstream frame once, including binary frames, and uses
the existing upstream request capture helper. It carries the diagnostic session
ID through the upstream's new local-interrupt error and response writers.
These changes do not alter interrupt routing, credential selection, or payloads.

The Codex session-context patch preserves `codex.response.metadata` for Codex model ETag updates.
When that event carries turn state, a separate, minimal `response.metadata`
event supplies it to the client. See the [metadata compatibility notes](codex-response-metadata.md)
for the captured evidence and transport contract.

Thread-title configuration now uses `upstream.codex.thread-title-session`.
Historical OAuth and legacy spellings keep their meaning. The default remains
allowed. Native model-list gating remains unchanged. Upstream's new
`GET /v1/models/*model` detail route retains its official behavior; this upgrade
does not extend the existing list gate to that new route.

The ignored materialized source is a build result. The infra translator's separate
SDK v7.3.20 pin is unchanged. The host uses official v8.0.33 plus the local ordered,
checksummed patches, with no parallel source fork.

## Temporary client workaround

If the native duplex route is unavailable, this Codex setting restores the 0.161
feature default:

```toml
[features]
instant_interrupt = false
```

It disables immediate preemption. This remains a client fallback when the native
WebSocket route is unavailable.

## Verification of the v8.0.22 integration

For the current rebase checks, see [v8.0.33 and API-key web search](cliproxyapi-v8.0.33-web-search.md).

The ordered 12-patch materialization check and CPA module tests passed. Official
interrupt tests passed for steering disabled and enabled, exact control bytes,
continuation on one socket, malformed and idle controls, disabled credentials,
HTTP cancellation, and Home session credentials. The diagnostic regression test
checks matching session attribution and interrupt visibility with request logging.
Native interrupt, Home, diagnostic, duplex, and steering race checks passed.

The complete generated upstream suite failed only its multicast discovery
integration test. Unmodified official v8.0.22 reproduced that failure in this
environment. The suite passed with only that multicast test excluded.
The broad interrupt race run also hit the official HTTP cancellation test's
immediate assertion after the incomplete frame. A repeated run on unmodified
upstream reproduced this timing failure. Neither run reported a data race.
Upstream production behavior and those tests remain unchanged.
