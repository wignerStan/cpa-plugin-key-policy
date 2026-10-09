# Short reasoning summaries in Codex

The development proxy captures explain the reported `I` and `I think it might be`
displayed next to agent tool activity on 2026-10-09. Both are the body of a
provider reasoning summary, rather than a final assistant answer.

| Captured summary | Upstream done event | Downstream done event | Response status |
| --- | --- | --- | --- |
| `I` | Same 36-character summary, including its heading | Same SHA-256 | completed |
| `I think it might be` | Same 54-character summary, including its heading | Same SHA-256 | completed |

Each upstream `response.reasoning_summary_text.done` already contains the short
body. Matching downstream events retain the same item ID, sequence number,
length, and SHA-256. The upstream and downstream `response.completed` events
contain the matching reasoning item and a `send_message` tool call. Neither
response has incomplete details. Thus the provider finalized each short summary
before the next tool action. CLIProxyAPI did not remove its remaining words.

A separate, later `response.interrupt` was forwarded correctly and ended a
different response with reason `interrupted`. That event does not explain these
two completed summaries. Another partial assistant message, `I’ll compare`,
also matches on both sides of the proxy, but has no captured text completion.
It must not be conflated with the completed short reasoning summaries.

Complete assistant progress messages in the same retained capture also have
matching delta text and done text on both sides. Raw capture copies data into a
separate collector; this evidence shows no response-text truncation by that dump
path. These observations identify where the fragments arrive, but do not prove
why the provider's summary generator ends mid-sentence.

No proxy text filter or synthesized completion was added. Such a filter would
hide or replace provider output and would not repair the summary generator. The
metadata compatibility fix is independent of these fragments.

Only short reported summary bodies and hashes are retained in the investigation
report. Credentials, turn-state values, and private raw payloads are excluded.
