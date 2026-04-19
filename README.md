# lm-agent-sdk-go

Go SDK for building agentic applications backed by a local [LM Studio](https://lmstudio.ai/) server — the OpenAI-compatible `/v1/*` surface plus LM Studio's native `/api/v0/*` endpoints for richer model metadata, performance stats, and llama.cpp-era sampling controls.

- Package: `lmsdk`
- Default backend: `http://127.0.0.1:1234/v1`

## Install

```bash
go get github.com/ethpandaops/lm-agent-sdk-go
```

## Configuration

The SDK resolves configuration from explicit options first, then environment variables, then defaults.

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `LM_BASE_URL` | LM Studio server base URL | `http://127.0.0.1:1234/v1` |
| `LM_API_KEY` | Bearer auth token (optional, only if your server enforces auth) | _(none)_ |
| `LM_MODEL` | Model name | _(none — must be set via env or `WithModel()`)_ |
| `LM_AGENT_SESSION_STORE_PATH` | Local session store directory | _(none)_ |

### Option Precedence

1. Explicit option (e.g. `WithBaseURL(...)`, `WithAPIKey(...)`, `WithModel(...)`)
2. Environment variable (`LM_BASE_URL`, `LM_API_KEY`, `LM_MODEL`)
3. Built-in default (where applicable)

### LM Studio server setup

Run the server via the Developer tab of the LM Studio desktop app, or headless:

```bash
lms server start
# or headless daemon
lms daemon up
```

API authentication is **off by default**. To enable bearer auth, toggle "Require Authentication" in Developer → Server Settings and generate a token in "Manage Tokens".

## Developer Workflow

The repo ships a sibling-style `Makefile`:

- `make test` runs race-enabled package tests with coverage output.
- `make test-integration` runs `./integration/...` with `-tags=integration`.
- `make audit` runs the aggregate quality gate.

Integration setup:

- Set `LM_BASE_URL` or default to `http://127.0.0.1:1234/v1`.
- Set `LM_MODEL` to a model loaded (or JIT-loadable) in LM Studio.
- Set `LM_API_KEY` if auth is enabled.
- Integration tests skip when the local LM Studio server is unavailable.

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"time"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for msg, err := range lmsdk.Query(
		ctx,
		lmsdk.Text("Write a two-line haiku about Go concurrency."),
		// lmsdk.WithModel("qwen/qwen3.6-35b-a3b"),
	) {
		if err != nil {
			panic(err)
		}

		if result, ok := msg.(*lmsdk.ResultMessage); ok && result.Result != nil {
			fmt.Println(*result.Result)
		}
	}
}
```

## Surface

- `Query(ctx, content, ...opts)` and `QueryStream(...)` return `iter.Seq2[Message, error]`.
- `NewClient()` exposes `Start`, `StartWithContent`, `StartWithStream`, `Query`, `ReceiveMessages`, `ReceiveResponse`, `Interrupt`, `SetPermissionMode`, `SetModel`, `ListModels`, `ListModelsResponse`, `GetMCPStatus`, `RewindFiles`, and `Close`.
- `Agent.Act(ctx, prompt, ...)` mirrors the `.act()` multi-round tool loop from the official `lmstudio-python`/`lmstudio-js` SDKs.
- `NativeChatCompletions(ctx, prompt, ...)` hits LM Studio's native `/api/v0/chat/completions` endpoint, returning `Stats` (tokens/sec, TTFT, stop reason, draft acceptance), `ModelInfo` (arch, quant, context length), and `Runtime` information alongside the usual assistant response.
- `UserMessageContent` is the canonical input shape. Use `Text(...)` for text-only calls and `Blocks(...)` with `ImageInput(...)`, `FileInput(...)`, `AudioInput(...)`, or `VideoInput(...)` for multimodal chat-completions requests.
- `WithSDKTools(...)` registers high-level in-process tools under `mcp__sdk__<name>`.
- `WithOnUserInput(...)` handles SDK-owned user-input prompts built on top of tool calling.
- `ListModels(...)` and `ListModelsResponse(...)` use LM Studio's native `/api/v0/models` discovery (falls back to `/v1/models` if unavailable).
- `StatSession(...)`, `ListSessions(...)`, and `GetSessionMessages(...)` operate on the SDK's local persisted session store.

## LM Studio-specific options

| Option | Effect |
|---|---|
| `WithTTL(duration)` | Sets `ttl` (seconds) — how long a JIT-loaded model stays resident after the request. |
| `WithDraftModel(id)` | Enables speculative decoding via a same-family draft model. |
| `WithMinP(v)` | llama.cpp `min_p` sampling. |
| `WithRepeatPenalty(v)` | llama.cpp `repeat_penalty`. |
| `WithStreamUsage(bool)` | Toggles `stream_options.include_usage` (defaults to true). |

## Model Discovery

- Discovery uses `/api/v0/models`, falling back to `/v1/models` if the native path is unavailable.
- `ModelInfo` exposes LM-Studio-specific fields (`Publisher`, `Quantization`, `CompatibilityType`, `State`, `Capabilities`) alongside the generic helpers such as `SupportsToolCalling()`, `SupportsStructuredOutput()`, `SupportsReasoning()`, `SupportsImageInput()`, `MaxContextLength()`, and parsed pricing helpers.

## Multimodal Input

```go
content := lmsdk.Blocks(
	lmsdk.TextInput("Describe these two screenshots."),
	lmsdk.ImageInput("https://example.com/before.png"),
	lmsdk.ImageInput("data:image/png;base64,..."),
)

for msg, err := range lmsdk.Query(ctx, content) {
	_ = msg
	_ = err
}
```

Image formats: JPEG, PNG, WebP. PDFs passed via `image_url` are rejected by LM Studio — pre-render to image client-side.

## Agent.Act

```go
agent := &lmsdk.Agent{
    Model:     "qwen/qwen3.6-35b-a3b",
    Tools:     []lmsdk.Tool{sqrtTool, addTool},
    MaxRounds: 6,
}
result, err := agent.Act(ctx, "Compute sqrt(144) then add 7.")
// result.Text: "The final number is 19."
// result.ToolCalls: 2
```

See [`examples/agent_act`](./examples/agent_act) for the full calculator demo.

## Session Semantics

Session APIs are local SDK APIs, not remote server sessions.

- They read from the SDK session store configured with `WithSessionStorePath(...)` or `LM_AGENT_SESSION_STORE_PATH`.
- They do not derive from chat `session_id`.
- They are independent of LM Studio's stateful `/api/v1/chat` endpoint.

## Observability

The SDK provides opt-in OpenTelemetry metrics and distributed tracing. When no provider is configured all recording is a pure noop — zero overhead.

### Options

| Option | Description |
|---|---|
| `WithMeterProvider(mp)` | Sets an OTel `metric.MeterProvider` for SDK metrics |
| `WithTracerProvider(tp)` | Sets an OTel `trace.TracerProvider` for SDK spans |
| `WithPrometheusRegisterer(reg)` | Convenience: creates an OTel MeterProvider backed by a Prometheus Registerer |

### Metrics

**GenAI semantic convention metrics:**

| Metric | Type | Description |
|---|---|---|
| `gen_ai.client.operation.duration` | Histogram (s) | Duration of query operations |
| `gen_ai.client.token.usage` | Counter | Token usage by type (input/output) |
| `gen_ai.client.time_to_first_token` | Histogram (s) | Time to first content token |
| `gen_ai.client.time_per_output_token` | Histogram (s) | Inter-token arrival time |

**SDK-specific metrics** (namespace: `lmstudio.*`):

| Metric | Type | Description |
|---|---|---|
| `lmstudio.http.requests` | Counter | HTTP requests by status class and retry |
| `lmstudio.tool.calls` | Counter | Tool calls by name and outcome |
| `lmstudio.tool.duration` | Histogram (s) | Tool call duration |
| `lmstudio.checkpoint.operations` | Counter | Checkpoint create/restore operations |
| `lmstudio.hook.duration` | Histogram (s) | Hook execution duration by event |

### Prometheus Example

```go
reg := prometheus.NewRegistry()

for msg, err := range lmsdk.Query(ctx,
    lmsdk.Text("Hello"),
    lmsdk.WithPrometheusRegisterer(reg),
    lmsdk.WithModel("qwen/qwen3.6-35b-a3b"),
) {
    // ...
}

// Serve metrics
http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
```

See [`examples/prometheus_observability`](./examples/prometheus_observability) for a complete working example.

## Known LM Studio quirks the SDK handles

- **Bug #619**: LM Studio serves the currently-loaded model regardless of the `model` field when only one is loaded. Validate via `ListModelsResponse` if you need strict model selection.
- **Bug #618**: A 200 OK may still carry `{"error": ...}`. The transport detects this and surfaces it as an error.
- **0.3.23 rename**: The `reasoning_content` field was renamed to `reasoning`. The event mapper accepts both.
- **Default 300s server timeout**: Prefer streaming for long completions.

## Examples

Runnable examples live under [`examples`](./examples). Highlights:

- `quick_start` — minimal one-shot query
- `client_multi_turn` — stateful client
- `agent_act` — multi-round autonomous tool loop
- `lmstudio_sampling` — LM Studio-flavoured sampling + TTL
- `lmstudio_native_stats` — `/api/v0` stats, model_info, runtime surface
- `mcp_calculator`, `sdk_tools`, `memory_tool` — MCP + in-process tools
- `sessions_local` — persist/resume SDK sessions
- `prometheus_observability` — OTel + Prometheus export
