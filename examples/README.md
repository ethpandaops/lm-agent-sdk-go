# Examples

This directory contains runnable examples for `github.com/ethpandaops/vllm-agent-sdk-go`.

## API Overview

The SDK supports two primary styles:

1. Top-level one-shot APIs:
- `Query(ctx, content, ...opts)`
- `QueryStream(ctx, messages, ...opts)`

2. Stateful client API:
- `NewClient()` + `Start()`
- `Query()` + `ReceiveResponse()` / `ReceiveMessages()`
- `Interrupt()` / `SetModel()` / `SetPermissionMode()`

## Environment

The core SDK resolves `LM_BASE_URL`, `LM_API_KEY`, and `LM_MODEL` automatically (see [Configuration](../README.md#configuration)). Examples add additional overrides:

| Variable | Description | Default |
|---|---|---|
| `LM_BASE_URL` | vLLM server base URL | `http://127.0.0.1:1234/v1` |
| `LM_API_KEY` | Bearer auth token (if your server enforces auth) | _(none)_ |
| `LM_MODEL` | Model name | `QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ` (example default) |
| `VLLM_IMAGE_MODEL` | Image-capable model | `QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ` (example default) |
| `VLLM_VISION_MODEL` | Vision model for multimodal input | Falls back to `VLLM_IMAGE_MODEL`, then `LM_MODEL` |
| `VLLM_IMAGE_OUTPUT_DIR` | Directory for saving generated images | _(none)_ |

Use `LM_MODEL` to pin a different served model when you need deterministic capability coverage.

## Core SDK Examples

These focus on the sibling-style SDK contract that downstream code consumes directly.

| Example | Description |
|---|---|
| `quick_start` | Basic one-shot query with low-cost model defaults. |
| `query_stream` | Streaming input via `QueryStream` and `MessagesFromSlice`. |
| `client_multi_turn` | Stateful `Client` usage over multiple turns. |
| `model_discovery` | List models and inspect free/tool/structured-output/image capabilities. |
| `structured_output` | Structured JSON output with `WithOutputFormat`. |
| `sdk_tools` | In-process SDK tools via `WithSDKTools(...)`. |
| `on_user_input` | SDK-owned user input prompts via `WithOnUserInput(...)`. |
| `permissions` | Tool permission denial handling via `WithCanUseTool(...)`. |
| `hooks` | Hook callbacks around tool execution. |
| `sessions_local` | Local session persistence, listing, stats, and message inspection. |
| `interrupt` | Client cancellation via `Interrupt()`. |
| `error_handling` | Typed `UnsupportedControlError` and `ErrSessionNotFound` handling. |
| `system_prompt` | System prompt configuration (default vs custom string vs preset). |
| `extended_thinking` | Extended thinking with `WithThinking` and `WithEffort`. |
| `include_partial_messages` | Real-time streaming of partial message deltas. |
| `max_budget_usd` | API cost control with `WithMaxBudgetUSD`. |
| `cancellation` | Context cancellation and graceful client shutdown. |
| `parallel_queries` | Concurrent `Query()` calls with `errgroup`. |
| `pipeline` | Multi-step LLM orchestration (Generate → Evaluate → Refine). |
| `mcp_calculator` | In-process MCP server with calculator tools via `CreateSdkMcpServer`. |
| `mcp_status` | Query MCP server connection status via `GetMCPStatus`. |
| `memory_tool` | Filesystem-backed persistent memory via MCP tools. |
| `custom_logger` | Bridge any logging library (logrus) to `WithLogger` via `slog.Handler`. |

## VLLM-Native Advanced Examples

These focus on VLLM-specific routing and request-shape controls.

| Example | Description |
|---|---|
| `vllm_chat_controls` | Sampling/tool controls (`top_p`, penalties, seed, stop, logprobs). |
| `vllm_routing` | Provider/plugins/route/session/trace controls. |
| `vllm_responses` | `/responses` mode with instructions/text config/service tier/truncation. |
| `vllm_responses_chaining` | Responses chaining with `previous_response_id` and prompt cache key. |
| `vllm_multimodal_input` | Multimodal chat-completions input with block-based text + image parts. |
| `vllm_multimodal_image` | Multimodal/image generation with generated image blocks saved to disk. |
| `vllm_extra` | Escape-hatch payload overrides via `WithVLLMExtra`. |

## Running

```bash
# Run any example
go run ./examples/quick_start
go run ./examples/vllm_responses
go run ./examples/vllm_multimodal_input

# Examples with sub-examples accept a name argument
go run ./examples/extended_thinking all
go run ./examples/cancellation graceful_shutdown
```

## Testing

```bash
# Run all examples and verify output with VLLM
scripts/test_examples.sh

# Run specific examples
scripts/test_examples.sh -f quick_start,pipeline

# Keep going on failure
scripts/test_examples.sh -k

# Override the model
LM_MODEL=QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ scripts/test_examples.sh

# Image-generation example with explicit image model and output directory
VLLM_IMAGE_MODEL=QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ \
VLLM_IMAGE_OUTPUT_DIR=/tmp/vllm-images \
go run ./examples/vllm_multimodal_image

# Multimodal input example with a vision-capable model
VLLM_VISION_MODEL=QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ \
go run ./examples/vllm_multimodal_input
```
