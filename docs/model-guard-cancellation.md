# Guard cancellation and private execution limits

The bound model callback must return caller cancellation as a callback error.
It must not return `upstream_failed` or a successful end of stream for a canceled
caller. The plugin stops the request and discards held output on callback errors.
It does not rotate the physical session, evict routing hints, charge a retry, or
disable the thread target for that cancellation.

The host checks the original bound context after Execute and Stream return.
It also checks that context after reading a stream chunk. This closes the race
where a queued error or closed channel wins the select against cancellation.
Terminal cleanup remains idempotent. Ordinary detached plugin streams retain
their existing lifetime.

Bound non-streaming auxiliary calls have a four-minute execution deadline. The
ModelTrace side and steer requests use this path. It bounds the upstream SSE
read performed by native Execute, including a response that never sends its
terminal event. The existing 8 MiB wire limit and 256 KiB auxiliary result limit
still apply. The parent request receives no new deadline or token limit.

The former 90-second cap interrupted legitimate high-effort responses. Four
minutes leaves time for local scoring inside the plugin's five-minute warmup
phase; three such calls also fit its fifteen-minute confirmation phase.

When only the private deadline expires, the host returns `auxiliary_timeout` as
an upstream observation. The plugin can retry within its existing thread budget.
When the caller cancels or reaches its own deadline, that caller error takes
priority. No timeout classification is inferred from upstream error text.

This changes host behavior only. It adds no SDK field or ABI method. Changes
replace the original owning patches in `plugin-host-wiring`.

`model_guard_cancellation_test.go` checks callback-return races, canceled reads
with queued errors or EOF, idempotent close, private timeout, caller deadline
priority, and unchanged parent deadlines. A two-minute successful auxiliary
response is a regression case. Deadline tests use Go virtual time.
