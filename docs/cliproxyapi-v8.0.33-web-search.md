# CLIProxyAPI v8.0.33 and API-key web search

The host now pins official CLIProxyAPI v8.0.33 at
`2422c6b0f5fe16495320fa29456493874c1977a0`. The local changes were rebased
from v8.0.22. Each semantic bundle's `apply_order.txt` contains its patch hashes.
The source relationship orders the bundles and records the final build tree.

The rebase retains upstream's subscription-usage filtering, Sign in with ChatGPT
configuration, and WebSocket reader completion and interrupt diagnostics.
Local interrupt capture now reads the WebSocket URL under the connection lock.

## Standard web search with API keys

**The Codex API-key provider can forward standard built-in web search.** OAuth
is not required by this proxy path. The configured upstream and model must
support the tool. This check did not call a live provider, so it does not establish
whether a particular remote account can execute search.

Send this body to the proxy's `POST /v1/responses`. Replace the model with an
allowed alias that routes to the Codex API-key credential:

```json
{
  "model": "YOUR_CODEX_ALIAS",
  "input": "Search the web for the official Go documentation and cite it.",
  "tools": [{"type": "web_search"}],
  "tool_choice": "required",
  "include": ["web_search_call.action.sources"]
}
```

`tool_choice: "required"` requires a tool call. Use `"auto"` to let the model
decide. Search is a built-in tool here; declaring a function named `web_search`
instead would require the client to execute that function.

The Codex executor reads `api_key` and `base_url` from the credential. It sends
the request to `base_url + /responses` with Bearer authentication. Configure the
base URL for the intended API-key upstream. If it is absent, this path defaults
to `https://chatgpt.com/backend-api/codex`; selecting API-key authentication does
not automatically select the public OpenAI API endpoint.

The Responses translator preserves `web_search` and converts
`web_search_preview` and `web_search_preview_2025_03_11` to `web_search`.
It also preserves the requested source list. Search results and URL citations
are retained in Responses output. The Chat Completions request translator also
passes built-in tools through, but this probe used the Responses interface.

## Separate alpha search endpoint

`POST /v1/alpha/search` has a separate credential policy. Ordinary API-key
credentials are excluded by default. A Codex API key must set
`api-keys.codex[].keys[].alpha-search: true`, and its upstream must support
`base-url + /alpha/search`. That flag is not required for the standard
`web_search` tool above.

## Verification

- `bash scripts/check-cliproxyapi.sh` passed. It reapplied all patches and tested
  the server, API, configuration, logging, plugin host, executors, Responses
  handlers, and service integration.
- `go build -o /tmp/cpa-v8.0.33-verified ./cmd/server` passed in the generated host.
- Both Codex-to-OpenAI translator package test suites passed.
- The auth package's `AlphaSearch` tests passed, including rejection of an
  ordinary API key on the alpha route.
- A temporary executor probe passed six local mock cases: the three search tool
  names above, each with streaming enabled and disabled. It used API-key auth
  without OAuth metadata or alpha-search opt-in. It checked the outgoing path,
  Bearer header, tool type, required tool choice, and source inclusion. It also
  checked that search results, citations, and source URLs reached the response.

These checks verify proxy behavior. They do not verify live search execution,
remote model availability, or account permissions.

## Source locations

Paths are relative to the generated `build/cliproxyapi` directory:

- `internal/runtime/executor/codex_executor_auth.go`: API-key credentials.
- `internal/runtime/executor/codex_executor_execute.go` and
  `codex_executor_stream.go`: HTTP request and response paths.
- `internal/translator/codex/openai/responses/`: built-in tool normalization,
  source inclusion, and result preservation.
- `internal/translator/codex/openai/chat-completions/codex_openai_request.go`:
  built-in tool forwarding.
- `sdk/cliproxy/auth/credential_policy.go`: alpha-search credential opt-in.
- `config.example.yaml`: Codex API-key configuration.
