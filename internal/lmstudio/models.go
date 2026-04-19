package lmstudio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/model"
)

type modelsEnvelope struct {
	Object string            `json:"object"`
	Data   []json.RawMessage `json:"data"`
}

type rawModel struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Type              string   `json:"type"`
	Publisher         string   `json:"publisher"`
	Arch              string   `json:"arch"`
	CompatibilityType string   `json:"compatibility_type"`
	Quantization      string   `json:"quantization"`
	State             string   `json:"state"`
	MaxContextLength  int      `json:"max_context_length"`
	LoadedContextLen  int      `json:"loaded_context_length,omitempty"`
	Capabilities      []string `json:"capabilities"`
	OwnedBy           string   `json:"owned_by"`
}

// ListModelsResponse queries LM Studio's /api/v0/models endpoint (richer
// metadata than the OpenAI-compat /v1/models surface). If that path is
// unreachable it falls back to /v1/models.
func ListModelsResponse(ctx context.Context, opts *config.Options) (*model.ListResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &config.Options{}
	}
	opts.ApplyDefaults()

	timeout := 60 * time.Second
	if opts.RequestTimeout != nil {
		timeout = *opts.RequestTimeout
	}
	client := &http.Client{Timeout: timeout}

	nativeBase := deriveNativeBase(ResolveBaseURL(opts))
	nativeURL, err := url.JoinPath(strings.TrimSuffix(nativeBase, "/"), "models")
	if err != nil {
		return nil, fmt.Errorf("join native models endpoint: %w", err)
	}

	envelope, endpointUsed, err := fetchModels(ctx, client, nativeURL, opts)
	if err != nil {
		// Fallback to OpenAI-compat /v1/models if native path fails.
		fallback, ferr := url.JoinPath(strings.TrimSuffix(ResolveBaseURL(opts), "/"), "models")
		if ferr != nil {
			return nil, fmt.Errorf("join fallback models endpoint: %w", ferr)
		}
		envelope, endpointUsed, err = fetchModels(ctx, client, fallback, opts)
		if err != nil {
			return nil, err
		}
	}

	out := &model.ListResponse{
		Object:        firstNonEmpty(envelope.Object, "list"),
		Source:        "lmstudio",
		RawData:       make([]model.Info, 0, len(envelope.Data)),
		Models:        make([]model.Info, 0, len(envelope.Data)),
		Total:         0,
		Authenticated: ResolveAPIKey(opts) != "",
		Endpoint:      endpointUsed,
	}

	for _, item := range envelope.Data {
		info, err := decodeModel(item)
		if err != nil {
			return nil, err
		}
		out.RawData = append(out.RawData, info)
		out.Models = append(out.Models, info)
	}
	out.Total = len(out.Models)
	return out, nil
}

func fetchModels(ctx context.Context, client *http.Client, endpointURL string, opts *config.Options) (*modelsEnvelope, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lm-agent-sdk-go/0.1.0")
	if apiKey := ResolveAPIKey(opts); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute models request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read models response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, "", fmt.Errorf("lmstudio models listing failed status=%d body=%s", res.StatusCode, string(body))
	}

	var envelope modelsEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, "", fmt.Errorf("decode models response: %w", err)
	}
	return &envelope, endpointURL, nil
}

func decodeModel(raw json.RawMessage) (model.Info, error) {
	var base rawModel
	if err := json.Unmarshal(raw, &base); err != nil {
		return model.Info{}, fmt.Errorf("decode model entry: %w", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return model.Info{}, fmt.Errorf("decode model metadata: %w", err)
	}

	contextLength := base.MaxContextLength
	if contextLength == 0 {
		contextLength = intFromMetadata(metadata, "context_length", "max_context_len", "max_model_len")
	}

	modality := "text"
	switch strings.ToLower(base.Type) {
	case "vlm":
		modality = "multimodal"
	case "embeddings":
		modality = "embedding"
	}

	capabilitySet := make(map[string]struct{}, len(base.Capabilities))
	for _, cap := range base.Capabilities {
		capabilitySet[strings.ToLower(strings.TrimSpace(cap))] = struct{}{}
	}
	_, hasToolUse := capabilitySet["tool_use"]
	_, hasReasoning := capabilitySet["reasoning"]
	_, hasVision := capabilitySet["vision"]
	if hasVision && modality == "text" {
		modality = "multimodal"
	}

	supported := model.SupportedParameters{
		"messages",
		"temperature",
		"top_p",
		"top_k",
		"min_p",
		"repeat_penalty",
		"max_tokens",
		"stop",
		"seed",
		"response_format",
	}
	if hasToolUse {
		supported = append(supported, "tools", "tool_choice", "parallel_tool_calls")
	}
	if hasReasoning {
		supported = append(supported, "reasoning")
	}

	info := model.Info{
		ID:                base.ID,
		Name:              firstNonEmpty(base.ID),
		Description:       strings.TrimSpace(base.OwnedBy),
		ContextLength:     contextLength,
		Publisher:         strings.TrimSpace(base.Publisher),
		Quantization:      strings.TrimSpace(base.Quantization),
		CompatibilityType: strings.TrimSpace(base.CompatibilityType),
		State:             strings.TrimSpace(base.State),
		Capabilities:      append([]string(nil), base.Capabilities...),
		Architecture: &model.Architecture{
			Modality:  modality,
			Tokenizer: strings.TrimSpace(base.Arch),
		},
		SupportedParameters: supported,
		Endpoints: []model.Endpoint{{
			Name:          "chat/completions",
			ContextLength: contextLength,
		}},
		DefaultEndpoint: "chat/completions",
		IsFree:          true,
		IsReasoning:     hasReasoning || looksReasoningModel(base.ID),
		Metadata:        metadata,
	}
	return info, nil
}

// looksReasoningModel is a name-heuristic fallback invoked when the
// /api/v0/models `capabilities` array doesn't advertise "reasoning" —
// LM Studio's v0 schema only surfaces a coarse capability list while the
// v1 endpoint carries the richer `capabilities.reasoning.allowed_options`
// object. Callers that want authoritative reasoning-mode metadata should
// use the v1 endpoint; this heuristic is a best-effort fallback so that
// ModelInfo.SupportsReasoning() returns true for the common families that
// ship with thinking enabled by default.
//
// Match strategy: lower-cased substring probes on the model ID, grouped by
// family so the intent is obvious at the call site:
//
//   - "r1" / "deepseek-r" — DeepSeek R1 + distills
//   - "reason" / "reasoning" — explicit name (e.g. mistral ministral-reasoning)
//   - "thinking" — phi-4-thinking, qwen3-thinking builds
//   - "qwen3" / "qwen4" / "qwen5" — Qwen3+ all ship with a thinking channel
//   - "o1" / "o3" / "o4" — OpenAI reasoning-series identifiers (hypothetical
//     for local hosting but cheap to match)
//   - "marco-o1", "gpt-oss" — community reasoning-tuned builds
func looksReasoningModel(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	reasoningSubstrings := []string{
		"reason", "thinking", "r1", "deepseek-r",
		"qwen3", "qwen4", "qwen5",
		"marco-o1", "gpt-oss",
	}
	for _, needle := range reasoningSubstrings {
		if strings.Contains(name, needle) {
			return true
		}
	}
	// OpenAI-style short prefix tags (o1, o3, o4, o1-mini, etc.). Match as
	// whole-word tokens to avoid false positives like "foo1" or "torio3".
	for _, token := range []string{"o1", "o3", "o4", "o5"} {
		if name == token ||
			strings.HasPrefix(name, token+"-") ||
			strings.HasPrefix(name, token+"_") ||
			strings.HasSuffix(name, "/"+token) ||
			strings.Contains(name, "/"+token+"-") ||
			strings.Contains(name, "/"+token+"_") {
			return true
		}
	}
	return false
}

func intFromMetadata(metadata map[string]any, keys ...string) int {
	for _, key := range keys {
		if raw, ok := metadata[key]; ok {
			switch v := raw.(type) {
			case float64:
				return int(v)
			case int:
				return v
			}
		}
	}
	return 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
