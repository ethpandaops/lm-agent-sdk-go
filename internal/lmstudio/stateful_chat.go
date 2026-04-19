package lmstudio

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
)

// ReasoningEffort controls the `reasoning` field on POST /api/v1/chat.
// LM Studio accepts a capability-filtered subset per model — some models
// only allow `"off"`/`"on"`, others the full low/medium/high ladder. Consult
// `capabilities.reasoning.allowed_options` from /api/v1/models.
type ReasoningEffort string

const (
	ReasoningOff    ReasoningEffort = "off"
	ReasoningLow    ReasoningEffort = "low"
	ReasoningMedium ReasoningEffort = "medium"
	ReasoningHigh   ReasoningEffort = "high"
	ReasoningOn     ReasoningEffort = "on"
)

// StatefulIntegration describes an entry in `integrations[]` on a stateful
// chat request — a per-request MCP server or plugin the model may call
// during this single response.
type StatefulIntegration struct {
	Type         string            `json:"type"`
	Name         string            `json:"name,omitempty"`
	ServerURL    string            `json:"server_url,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	AllowedTools []string          `json:"allowed_tools,omitempty"`
}

// StatefulChatRequest is the body of POST /api/v1/chat.
type StatefulChatRequest struct {
	Model string `json:"model"`

	// Input is the user text or a structured input array. Required unless
	// PreviousResponseID continues a stored conversation.
	Input any `json:"input,omitempty"`

	// PreviousResponseID resumes a prior response's conversation state
	// (requires Store=true on the earlier call).
	PreviousResponseID string `json:"previous_response_id,omitempty"`

	// Instructions prepend a system prompt to the turn.
	Instructions string `json:"instructions,omitempty"`

	// Reasoning sets the effort level. See ReasoningEffort.
	Reasoning ReasoningEffort `json:"reasoning,omitempty"`

	// Store persists this response server-side so a later call can reference
	// it via PreviousResponseID.
	Store *bool `json:"store,omitempty"`

	// Temperature / TopP / TopK / MinP / RepeatPenalty mirror the sampling
	// surface from /v1/chat/completions.
	Temperature   *float64 `json:"temperature,omitempty"`
	TopP          *float64 `json:"top_p,omitempty"`
	TopK          *float64 `json:"top_k,omitempty"`
	MinP          *float64 `json:"min_p,omitempty"`
	RepeatPenalty *float64 `json:"repeat_penalty,omitempty"`
	Seed          *int64   `json:"seed,omitempty"`

	// Integrations registers ephemeral MCP servers or plugins for this turn.
	Integrations []StatefulIntegration `json:"integrations,omitempty"`
	// AllowedTools filters available tools (from integrations + global).
	AllowedTools []string `json:"allowed_tools,omitempty"`

	// Stream flips SSE streaming on. Defaults to non-streaming.
	Stream bool `json:"stream,omitempty"`

	// Extra merges raw fields into the outgoing payload as an escape hatch.
	Extra map[string]any `json:"-"`
}

// StatefulOutput is one element of the `output[]` array in the response.
type StatefulOutput struct {
	Type      string          `json:"type"`
	Content   string          `json:"content,omitempty"`
	Reasoning string          `json:"reasoning,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	ToolCall  map[string]any  `json:"tool_call,omitempty"`
	Meta      map[string]any  `json:"-"`
}

// StatefulStats is the stats block on a stateful chat response.
type StatefulStats struct {
	InputTokens             int     `json:"input_tokens"`
	TotalOutputTokens       int     `json:"total_output_tokens"`
	ReasoningOutputTokens   int     `json:"reasoning_output_tokens"`
	TokensPerSecond         float64 `json:"tokens_per_second"`
	TimeToFirstTokenSeconds float64 `json:"time_to_first_token_seconds"`
	ModelLoadTimeSeconds    float64 `json:"model_load_time_seconds,omitempty"`
	StopReason              string  `json:"stop_reason,omitempty"`
}

// StatefulChatResponse is the full response from POST /api/v1/chat (non-streaming).
type StatefulChatResponse struct {
	ResponseID      string           `json:"response_id"`
	ModelInstanceID string           `json:"model_instance_id"`
	Output          []StatefulOutput `json:"output"`
	Stats           StatefulStats    `json:"stats"`
}

// Text returns the concatenated text of all `message` outputs, skipping
// reasoning and tool-call entries. Convenience for callers that just want
// the final assistant reply.
func (r *StatefulChatResponse) Text() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, item := range r.Output {
		if item.Type == "message" && item.Content != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(item.Content)
		}
	}
	return b.String()
}

// Reasoning returns the concatenated reasoning text from `reasoning`-typed
// output entries, if any were captured.
func (r *StatefulChatResponse) ReasoningText() string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	for _, item := range r.Output {
		if item.Reasoning != "" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(item.Reasoning)
		}
	}
	return b.String()
}

// StatefulChat posts a stateful native chat request. Unlike
// /v1/chat/completions this endpoint:
//
//   - Takes `input` (string or content array) rather than `messages[]`.
//   - Supports `reasoning` effort as a first-class string.
//   - Returns a `response_id` that can be threaded into follow-up calls via
//     `previous_response_id` when `store: true` was set.
//   - Supports per-request MCP via `integrations[]`.
//   - Surfaces richer stats (`reasoning_output_tokens`, TTFT, model load time).
//
// Pass `Stream: true` on the request to use StatefulChatStream instead.
func StatefulChat(ctx context.Context, opts *config.Options, req StatefulChatRequest) (*StatefulChatResponse, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("StatefulChat: model is required")
	}
	if req.Input == nil && strings.TrimSpace(req.PreviousResponseID) == "" {
		return nil, fmt.Errorf("StatefulChat: input or previous_response_id is required")
	}
	req.Stream = false

	body := buildStatefulChatBody(req)
	var resp StatefulChatResponse
	if err := postJSON(ctx, opts, "chat", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func buildStatefulChatBody(req StatefulChatRequest) map[string]any {
	b := map[string]any{"model": strings.TrimSpace(req.Model)}
	if req.Input != nil {
		b["input"] = req.Input
	}
	if req.PreviousResponseID != "" {
		b["previous_response_id"] = req.PreviousResponseID
	}
	if req.Instructions != "" {
		b["instructions"] = req.Instructions
	}
	if req.Reasoning != "" {
		b["reasoning"] = string(req.Reasoning)
	}
	if req.Store != nil {
		b["store"] = *req.Store
	}
	if req.Temperature != nil {
		b["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		b["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		b["top_k"] = *req.TopK
	}
	if req.MinP != nil {
		b["min_p"] = *req.MinP
	}
	if req.RepeatPenalty != nil {
		b["repeat_penalty"] = *req.RepeatPenalty
	}
	if req.Seed != nil {
		b["seed"] = *req.Seed
	}
	if len(req.Integrations) > 0 {
		b["integrations"] = req.Integrations
	}
	if len(req.AllowedTools) > 0 {
		b["allowed_tools"] = req.AllowedTools
	}
	if req.Stream {
		b["stream"] = true
	}
	for k, v := range req.Extra {
		b[k] = v
	}
	return b
}
