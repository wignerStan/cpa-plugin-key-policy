# Codex 0.162 interruption compatibility

Codex 0.162 enables immediate interruption by default. A proxy that rejects
`response.interrupt` can therefore fail when new input steers an active turn.
The default change is specific to 0.162; the wire message already existed in
0.161. Patch 0013 forwards it through the existing native Codex duplex executor.

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

## Required native route

The pinned v8 host enables its existing concurrent reader through the OAuth
provider setting:

```yaml
oauth:
  providers:
    codex:
      response-steering: true
```

This is an opt-in setting. Patch 0013 leaves its default unchanged. It also
requires a native Codex WebSocket credential and route. API-key groups have
separate provider options and remain subject to the host's existing eligibility
rules. Legacy configuration uses `codex.response-steering` before migration.
Plugin executor and HTTP-only routes do not gain active-response interruption
from this patch. A serial handler cannot interrupt an active response merely by
accepting the event type in its normalizer.

## Patch behavior

Interrupt and the existing steer control share the native control writer. Each
write checks that the socket's bound credential remains enabled. It keeps the
original payload, connection, credential, logging path, and native write-error
handling. Interrupt does not enter create translation, history reconstruction,
or automatic steer-successor state.

The upstream owns acknowledgements, discarded-item events, terminal reasons,
and continuation history. The patch does not synthesize responses, map interrupt
to steer, select another credential, or replay an accepted turn. It adds no new
transport, pool, plugin callback, or payload parse tree.

The patch is stored in the ordered, checksummed CLIProxyAPI patch set. The ignored
materialized source is a build result. The infra translator separately pins SDK
v7.3.20; this host patch uses the v8 commit in its manifest.

## Temporary client workaround

If the native duplex route is unavailable, this Codex setting restores the 0.161
feature default:

```toml
[features]
instant_interrupt = false
```

It disables immediate preemption. No live configuration or deployed service was
changed during this work.

## Verification

The native executor tests passed, including the active-response interrupt
lifecycle and revoked-credential rejection. The full Gin/auth/executor test
passed with the exact v8 OAuth configuration above. It checks exact control
bytes, upstream acknowledgement and discarded-item event order, the incomplete
boundary with terminal usage, and an explicit client continuation on one socket.
The handler retains its existing normalization of missing usage detail fields.

All existing duplex executor and steering handler tests passed. Race checks
passed for the duplex executor tests and both full-route integration tests.
The ordered manifest regenerated successfully with 13 patches, and
`scripts/check-cliproxyapi.sh` passed. The native server compiled successfully.
The check script now includes the executor and downstream OpenAI handler packages.
