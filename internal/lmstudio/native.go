package lmstudio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
)

// NativeStats mirrors LM Studio's /api/v0 response `stats` object.
type NativeStats struct {
	TokensPerSecond          float64 `json:"tokens_per_second"`
	TimeToFirstToken         float64 `json:"time_to_first_token"`
	GenerationTime           float64 `json:"generation_time"`
	StopReason               string  `json:"stop_reason"`
	DraftModel               string  `json:"draft_model,omitempty"`
	TotalDraftTokensCount    int     `json:"total_draft_tokens_count,omitempty"`
	AcceptedDraftTokensCount int     `json:"accepted_draft_tokens_count,omitempty"`
	RejectedDraftTokensCount int     `json:"rejected_draft_tokens_count,omitempty"`
	IgnoredDraftTokensCount  int     `json:"ignored_draft_tokens_count,omitempty"`
}

// NativeModelInfo mirrors LM Studio's /api/v0 response `model_info` object.
type NativeModelInfo struct {
	Arch          string `json:"arch"`
	Quant         string `json:"quant"`
	Format        string `json:"format"`
	ContextLength int    `json:"context_length"`
}

// NativeRuntime mirrors LM Studio's /api/v0 response `runtime` object.
type NativeRuntime struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	SupportedFormats []string `json:"supported_formats"`
}

// NativeChoice mirrors a single element of the native response `choices` array.
type NativeChoice struct {
	Index        int              `json:"index"`
	Message      NativeMessage    `json:"message"`
	FinishReason string           `json:"finish_reason"`
	Logprobs     *json.RawMessage `json:"logprobs,omitempty"`
}

// NativeMessage is an assistant message returned by /api/v0.
type NativeMessage struct {
	Role             string            `json:"role"`
	Content          string            `json:"content"`
	ReasoningContent string            `json:"reasoning_content,omitempty"`
	Reasoning        string            `json:"reasoning,omitempty"`
	ToolCalls        []json.RawMessage `json:"tool_calls,omitempty"`
}

// NativeUsage mirrors OpenAI-compat usage with reasoning extras.
type NativeUsage struct {
	PromptTokens            int `json:"prompt_tokens"`
	CompletionTokens        int `json:"completion_tokens"`
	TotalTokens             int `json:"total_tokens"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

// NativeResponse is the deserialized response from /api/v0/chat/completions.
type NativeResponse struct {
	ID        string          `json:"id"`
	Object    string          `json:"object"`
	Created   int64           `json:"created"`
	Model     string          `json:"model"`
	Choices   []NativeChoice  `json:"choices"`
	Usage     NativeUsage     `json:"usage"`
	Stats     NativeStats     `json:"stats"`
	ModelInfo NativeModelInfo `json:"model_info"`
	Runtime   NativeRuntime   `json:"runtime"`
}

// NativeChatCompletions calls POST /api/v0/chat/completions (non-streaming)
// and surfaces the extra LM Studio native fields.
func NativeChatCompletions(ctx context.Context, opts *config.Options, req *config.ChatRequest) (*NativeResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request is nil")
	}

	body := buildNativeBody(req)
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal native chat request: %w", err)
	}

	baseURL := deriveNativeBase(ResolveBaseURL(opts))
	u, err := url.JoinPath(strings.TrimSuffix(baseURL, "/"), "chat/completions")
	if err != nil {
		return nil, fmt.Errorf("join native chat url: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "lm-agent-sdk-go/0.1.0")
	if apiKey := ResolveAPIKey(opts); apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := nativeHTTPClient(opts)
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read native response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("lmstudio native status=%d body=%s", resp.StatusCode, string(raw))
	}

	// 200 OK can still carry {"error": ...} per LM Studio bug #618.
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("decode native response: %w", err)
	}
	if errVal, ok := probe["error"]; ok {
		return nil, fmt.Errorf("lmstudio native error: %v", errVal)
	}

	var out NativeResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode native response: %w", err)
	}
	return &out, nil
}

func buildNativeBody(req *config.ChatRequest) map[string]any {
	body := map[string]any{
		"model":    strings.TrimSpace(req.Model),
		"messages": cloneMapSlice(req.Messages),
		"stream":   false,
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		if req.ToolChoice != nil {
			body["tool_choice"] = req.ToolChoice
		} else {
			body["tool_choice"] = "auto"
		}
	}
	if req.MaxTokens != nil {
		body["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		body["top_p"] = *req.TopP
	}
	if req.TopK != nil {
		body["top_k"] = *req.TopK
	}
	if req.MinP != nil {
		body["min_p"] = *req.MinP
	}
	if req.RepeatPenalty != nil {
		body["repeat_penalty"] = *req.RepeatPenalty
	}
	if req.PresencePenalty != nil {
		body["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		body["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.Seed != nil {
		body["seed"] = *req.Seed
	}
	if len(req.Stop) > 0 {
		if len(req.Stop) == 1 {
			body["stop"] = req.Stop[0]
		} else {
			body["stop"] = cloneStringSlice(req.Stop)
		}
	}
	if len(req.ResponseFormat) > 0 {
		body["response_format"] = normalizeChatResponseFormat(req.ResponseFormat)
	}
	if req.TTL != nil {
		body["ttl"] = *req.TTL
	}
	if strings.TrimSpace(req.DraftModel) != "" {
		body["draft_model"] = strings.TrimSpace(req.DraftModel)
	}
	if len(req.Extra) > 0 {
		mergeInto(body, req.Extra)
	}
	return body
}

// deriveNativeBase converts a `/v1` base URL into its `/api/v0` sibling.
func deriveNativeBase(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return strings.TrimSuffix(base, "/v1") + "/api/v0"
	}
	if strings.HasSuffix(base, "/api/v0") {
		return base
	}
	return base + "/api/v0"
}

func nativeHTTPClient(opts *config.Options) *http.Client {
	timeout := 2 * time.Minute
	if opts != nil && opts.RequestTimeout != nil && *opts.RequestTimeout > 0 {
		timeout = *opts.RequestTimeout
	}
	return &http.Client{Timeout: timeout}
}
