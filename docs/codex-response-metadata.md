# Codex response metadata compatibility

Patch 0004 preserves the original `codex.response.metadata` event. If that
event contains a usable `x-codex-turn-state` header, it also sends a small
`response.metadata` event immediately afterward. The supplement contains only
the event type and the turn-state header. It does not copy model catalog,
moderation, or unknown metadata into a different event namespace.

## Why both event types matter

Codex 0.162 treats these event names differently. Its
[WebSocket reader](https://github.com/openai/codex/blob/rust-v0.162.0/codex-rs/codex-api/src/endpoint/responses_websocket.rs)
reads `x-models-etag` from `codex.response.metadata`. Its
[turn-state accessor](https://github.com/openai/codex/blob/rust-v0.162.0/codex-rs/codex-api/src/sse/responses.rs)
reads `x-codex-turn-state` only from `response.metadata`. The current upstream
reader inspected on 2026-10-09 retains that distinction. Renaming the original
event fixes turn-state capture but prevents event-based model ETag updates.

HTTP/SSE uses the HTTP response header to set turn state. A body event does not
replace that header path. The existing official-Codex SSE path preserves its
original metadata frame and adds the same supplement when needed; non-Codex
private-event filtering remains unchanged.

## Captured evidence

The live development dump directory was inspected without printing payloads or
turn-state values. An initial scan found 99 retained upstream
`codex.response.metadata` events, each with a model ETag. Four also carried turn
state. Matching downstream captures showed the old patch had renamed those
events to `response.metadata`.

A separate saved snapshot at 2026-10-09 12:05:58 UTC selected 446 modern named
WebSocket artifacts. It contained 34 upstream metadata events and 34 matching
downstream metadata events. Four in each direction carried turn state, with
matching SHA-256 fingerprints. The other 30 carried no turn state. Two sessions
had turn state in their first retained upstream metadata event. Earlier records
for another session had already expired, so these captures cannot establish the
first event for every complete socket lifetime.

The saved evidence contains counts, capture file hashes, header names, and
turn-state hashes. Raw dump artifacts remain private and outside the repository.
These counts describe a retained snapshot, not a durable traffic total.

## Wire and performance contract

- Forward the original metadata bytes, including the model ETag and extension
  fields, without rewriting them at the response writer. Existing upstream
  framing, including outer-whitespace trimming before that writer, is unchanged.
- Add the supplement only for a nonempty string state or an array whose first
  element resolves to a nonempty string, as the Codex parser expects. Header
  names are case-insensitive.
- Forward an existing `response.metadata` event unchanged, without adding a
  second event.
- Hold one WebSocket writer lock across the original and its supplement so
  another writer cannot separate them. Capture each actual outgoing frame.
- Add no supplemental event or payload copy for delta frames, ETag-only
  metadata, or unrelated events.

Patch 0004 also retains execution-session context and credential WebSocket
attributes used by the later diagnostic and transport patches. It cannot be
dropped as one unit solely because the client recognizes `response.metadata`.

## Verification and limits

The required materialization check passed all eight selected host packages. CPA
module tests and focused metadata race tests passed. TCP tests exercise the real
WebSocket handler, auth manager, and native executor. A direct writer test checks
incoming bytes, timeline entries, and raw capture for both outgoing frames. The
supplement helper allocates nothing for the tested delta input.

Metadata SSE frames wait for their delimiter instead of being emitted while
their data field is incomplete. A narrow guard also prevents line-break repair
inside an unfinished metadata JSON body. Tests split these bodies at every byte.
Existing fragmentation limits inside SSE field names and data-only recovery are
unchanged; this does not establish arbitrary-fragment correctness for every SSE
event. The original and supplemental WebSocket frames remain separate messages.
