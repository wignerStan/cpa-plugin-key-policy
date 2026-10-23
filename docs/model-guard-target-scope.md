# ModelGuard target scope

The host applies ModelGuard only to native `codex` credentials whose `AuthKind()`
is `apikey`. OAuth, SIWC, unknown credential kinds, other providers, and managed
PAYG credentials bypass this gate. The plugin wire protocol and C ABI do not
change.

Configure optional exact selectors under
`routing.cooldown.codex-thread.guard-targets`:

```yaml
guard-prefixes: [lab]
guard-targets:
  - auth-ids: ["codex:apikey:target-a", "codex:apikey:target-b"]
    base-urls: ["https://upstream.example/v1"]
    models: ["lab/gpt-6.1-sol", "lab/gpt-6-astra"]
  - auth-ids: ["codex:apikey:target-c"]
    models: ["lab/gpt-6-astra"]
```

Nonempty fields within a rule use AND. Values and rules use OR. Model names
include the native prefix. AuthIDs are runtime credential IDs, not config group
names. URL matching normalizes host case and trailing slashes only. It preserves
ports and paths. Credentials, query strings, fragments, and wildcard patterns
are invalid. Empty rules fail startup. Omitted targets cover all native Codex
API keys in the configured prefixes. `guard-prefixes: [""]` covers routes with
no prefix. Restart CPA after changing these startup bindings.

Selection and dispatch ignore guard thread exclusions for unmatched credentials
and models. An unmatched ordinary candidate also prevents a false PAYG pool
exhaustion decision. Independent thread cooldown without ModelGuard retains its
existing scope. Hourly inspection has its own explicit plugin target list; keep
that list restricted to the same API-key targets, or leave it empty.

Validation: complete auth-manager, API-handler, and config package tests; focused
race tests; plugin race tests; 55 offline E2E tests; the real shared-library C ABI
contract; and clean application of all 135 patches to v8.0.33.

The native Codex `gpt-6.1-sol` E2E passed with exact URL/model selection. The first
parent had 15,132 input tokens. One side sample hit 14,976 cache tokens and scored
0.911445 against the configured `gpt-6-astra` surrogate. The real tool-result
request also hit 14,976 cache tokens and completed. All three upstream responses
had consistent attribution and no BPS marker. This tests the guard path; the
surrogate fingerprint does not distinguish `gpt-6.1-sol` from `gpt-6-astra`.
