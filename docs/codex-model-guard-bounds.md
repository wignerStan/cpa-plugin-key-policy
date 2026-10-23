# Codex model guard response bounds

Native Codex nonstream execution reads an SSE response before returning the
assembled response JSON. These are two different sizes. A real native Lite
fingerprint capture contained 309,687 bytes of SSE while its completed JSON fit
within 256 KiB. Applying the JSON ceiling to the SSE body rejected valid replies.

The guarded request and periodic single-attempt inspection now use these limits:

| Object | Maximum size |
| --- | ---: |
| Held main response body / SSE read | 8 MiB |
| Auxiliary steer or inspection SSE read | 8 MiB |
| Assembled auxiliary steer or inspection JSON | 256 KiB |

The limits bound response payloads and temporary copies. They do not define a
process RSS ceiling. Request admission, cancellation and existing hold quotas
still apply. Inspection remains pinned to one auth and one execution attempt.

A local read, assembly or stream bound failure returns an execution callback
error. It must not become `upstream_failed` or `upstream_execution_failed`.
Those categories feed model retry and inspection decisions. Local buffer
exhaustion does not establish a failed model fingerprint and must not disable a
provider or spend the thread's retry budget.

This applies to non-2xx error bodies too. The native executor retains the typed
read-limit error instead of replacing it with an HTTP status error. The native
execution manager stops on that error before refresh, failure accounting or
cooldown updates, including after an unauthorized-response refresh attempt.
For assembled-result overflow, the upstream execution may already count as a
success; the local result-size check still must not add a failure.

The internal stream bridge preserves the local bound category through a private
marker. An upstream error with identical text remains an upstream error. No
public SDK field, C ABI field or plugin JSON schema changes.

Regression tests cover native SSE larger than 256 KiB with small final JSON,
wire data above 8 MiB, final JSON above 256 KiB, stream cancellation and cleanup,
and ordinary upstream error classification. Changes replace the existing owning
patches in `thread-admission` and `plugin-host-wiring`.
