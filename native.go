package lmsdk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/lmstudio"
)

// NativeStats surfaces LM Studio's `/api/v0` response `stats` object,
// exposing runtime performance metrics like tokens-per-second and
// time-to-first-token. When speculative decoding is enabled the draft
// acceptance counters are also populated.
type NativeStats = lmstudio.NativeStats

// NativeModelInfo surfaces LM Studio's `/api/v0` response `model_info`
// object with architecture, quantization, and context length.
type NativeModelInfo = lmstudio.NativeModelInfo

// NativeRuntime surfaces LM Studio's `/api/v0` response `runtime` object
// describing the llama.cpp/MLX engine build in use.
type NativeRuntime = lmstudio.NativeRuntime

// NativeResponse is the full response returned by NativeChatCompletions.
type NativeResponse = lmstudio.NativeResponse

// NativeChoice is a single choice in a NativeResponse.
type NativeChoice = lmstudio.NativeChoice

// NativeMessage is an assistant message inside a NativeChoice.
type NativeMessage = lmstudio.NativeMessage

// NativeUsage is the usage block inside a NativeResponse.
type NativeUsage = lmstudio.NativeUsage

// ReasoningEffort controls the `reasoning` field on StatefulChat requests.
type ReasoningEffort = lmstudio.ReasoningEffort

// Reasoning effort levels. Availability per model is advertised in
// `capabilities.reasoning.allowed_options` on /api/v1/models.
const (
	ReasoningOff    = lmstudio.ReasoningOff
	ReasoningLow    = lmstudio.ReasoningLow
	ReasoningMedium = lmstudio.ReasoningMedium
	ReasoningHigh   = lmstudio.ReasoningHigh
	ReasoningOn     = lmstudio.ReasoningOn
)

// StatefulChatRequest is the body of a POST /api/v1/chat call.
type StatefulChatRequest = lmstudio.StatefulChatRequest

// StatefulChatResponse is the response from StatefulChat.
type StatefulChatResponse = lmstudio.StatefulChatResponse

// StatefulOutput is one element of a stateful chat response's output[] list.
type StatefulOutput = lmstudio.StatefulOutput

// StatefulStats is the stats block on a stateful chat response.
type StatefulStats = lmstudio.StatefulStats

// StatefulIntegration configures a per-request MCP server or plugin for
// StatefulChat via `integrations[]`.
type StatefulIntegration = lmstudio.StatefulIntegration

// StatefulChat calls LM Studio's native stateful chat endpoint
// (POST /api/v1/chat). Unlike the OpenAI-compatible path, this endpoint:
//
//   - Takes `input` (string or content array) instead of `messages[]`.
//   - Supports a first-class `reasoning` effort knob (off/low/medium/high/on).
//   - Can thread conversation state via `previous_response_id` when the
//     prior turn was created with Store=true — no need to send history back.
//   - Accepts per-request MCP integrations via `integrations[]`.
//   - Returns richer stats: `reasoning_output_tokens`, TTFT, model load time.
//
// This is a one-shot non-streaming call. Use the main Query/QueryStream
// surface for OpenAI-compat streaming with MCP server registration.
func StatefulChat(ctx context.Context, req StatefulChatRequest, opts ...Option) (*StatefulChatResponse, error) {
	agentOpts := applyAgentOptions(opts)
	agentOpts.ApplyDefaults()
	if agentOpts.RequestTimeout == nil {
		t := 5 * time.Minute
		agentOpts.RequestTimeout = &t
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = agentOpts.Model
	}
	return lmstudio.StatefulChat(ctx, agentOpts, req)
}

// LoadConfig mirrors the LM Studio `/api/v1/models/load` configuration
// surface. Fields are passthrough — see internal/lmstudio.LoadConfig for
// per-field docs.
type LoadConfig = lmstudio.LoadConfig

// LoadRequest is the request body for LoadModel.
type LoadRequest = lmstudio.LoadRequest

// LoadResponse describes a successfully loaded model instance.
type LoadResponse = lmstudio.LoadResponse

// DownloadRequest is the request body for DownloadModel.
type DownloadRequest = lmstudio.DownloadRequest

// DownloadResponse is returned by DownloadModel with the job ID for polling.
type DownloadResponse = lmstudio.DownloadResponse

// DownloadStatus reports progress for an in-flight download.
type DownloadStatus = lmstudio.DownloadStatus

// LoadModel loads a model into LM Studio via POST /api/v1/models/load.
// Returns the loaded instance ID, which is required to later call
// UnloadModel.
func LoadModel(ctx context.Context, req LoadRequest, opts ...Option) (*LoadResponse, error) {
	agentOpts := applyAgentOptions(opts)
	agentOpts.ApplyDefaults()
	if agentOpts.RequestTimeout == nil {
		// Loading a 30B+ model can take 30+ seconds on cold disk; give it room.
		t := 5 * time.Minute
		agentOpts.RequestTimeout = &t
	}
	return lmstudio.LoadModel(ctx, agentOpts, req)
}

// UnloadModel unloads a previously-loaded instance via
// POST /api/v1/models/unload. The ID is the value returned from LoadModel or
// present in ListModelsResponse's loaded-instance payload.
func UnloadModel(ctx context.Context, instanceID string, opts ...Option) error {
	agentOpts := applyAgentOptions(opts)
	agentOpts.ApplyDefaults()
	return lmstudio.UnloadModel(ctx, agentOpts, instanceID)
}

// DownloadModel starts an async download from LM Studio's catalog or
// Hugging Face via POST /api/v1/models/download. Returns the job ID; poll
// DownloadStatusFor to watch progress.
func DownloadModel(ctx context.Context, req DownloadRequest, opts ...Option) (*DownloadResponse, error) {
	agentOpts := applyAgentOptions(opts)
	agentOpts.ApplyDefaults()
	return lmstudio.DownloadModel(ctx, agentOpts, req)
}

// DownloadStatusFor polls a download job's progress via
// GET /api/v1/models/download/status?job_id=...
func DownloadStatusFor(ctx context.Context, jobID string, opts ...Option) (*DownloadStatus, error) {
	agentOpts := applyAgentOptions(opts)
	agentOpts.ApplyDefaults()
	return lmstudio.DownloadStatusFor(ctx, agentOpts, jobID)
}

// NativeChatCompletions sends a non-streaming chat completion to LM Studio's
// native `/api/v0/chat/completions` endpoint. Unlike the OpenAI-compatible
// path, the response includes Stats (tokens/sec, TTFT), ModelInfo
// (arch, quant), and Runtime information — useful for observability and
// benchmarking without wiring OpenTelemetry.
func NativeChatCompletions(ctx context.Context, prompt string, opts ...Option) (*NativeResponse, error) {
	agentOpts := applyAgentOptions(opts)
	agentOpts.ApplyDefaults()

	model := strings.TrimSpace(agentOpts.Model)
	if model == "" {
		return nil, fmt.Errorf("model is required: set LM_MODEL or use WithModel")
	}

	messages := make([]map[string]any, 0, 2)
	if sp := strings.TrimSpace(agentOpts.SystemPrompt); sp != "" {
		messages = append(messages, map[string]any{"role": "system", "content": sp})
	}
	messages = append(messages, map[string]any{"role": "user", "content": prompt})

	req := &config.ChatRequest{
		Model:            model,
		Messages:         messages,
		Temperature:      agentOpts.Temperature,
		TopP:             agentOpts.TopP,
		TopK:             agentOpts.TopK,
		MinP:             agentOpts.MinP,
		RepeatPenalty:    agentOpts.RepeatPenalty,
		MaxTokens:        agentOpts.MaxTokens,
		PresencePenalty:  agentOpts.PresencePenalty,
		FrequencyPenalty: agentOpts.FrequencyPenalty,
		Seed:             agentOpts.Seed,
		Stop:             agentOpts.Stop,
		ResponseFormat:   agentOpts.OutputFormat,
		DraftModel:       agentOpts.DraftModel,
		Extra:            agentOpts.Extra,
		User:             agentOpts.User,
	}
	if agentOpts.TTL != nil {
		ttl := int(agentOpts.TTL.Seconds())
		if ttl > 0 {
			req.TTL = &ttl
		}
	}
	if agentOpts.RequestTimeout == nil {
		t := 5 * time.Minute
		agentOpts.RequestTimeout = &t
	}

	return lmstudio.NativeChatCompletions(ctx, agentOpts, req)
}
