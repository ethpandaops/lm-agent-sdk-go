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
	"sync"
	"time"

	"github.com/ethpandaops/agent-sdk-observability/semconv/httpconv"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/observability"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/util"
)

// HTTPTransport implements config.Transport against an LM Studio OpenAI-compatible server.
type HTTPTransport struct {
	opts    *config.Options
	client  *http.Client
	apiKey  string
	baseURL string
	obs     *observability.Observer

	mu      sync.Mutex
	started bool
}

// NewHTTPTransport creates an LM Studio HTTP transport.
func NewHTTPTransport(opts *config.Options) *HTTPTransport {
	// For streaming SSE connections the HTTP client timeout must not cap the
	// total body-read time — the context deadline already handles cancellation.
	// Default to no hard HTTP timeout; honour WithRequestTimeout when set.
	var timeout time.Duration
	if opts != nil && opts.RequestTimeout != nil {
		timeout = *opts.RequestTimeout
	}
	return &HTTPTransport{
		opts: opts,
		obs:  observability.Noop(),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// SetObserver sets the observability observer for the transport.
func (t *HTTPTransport) SetObserver(obs *observability.Observer) {
	if obs != nil {
		t.obs = obs
	}
}

// Start initializes transport state.
func (t *HTTPTransport) Start(_ context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started {
		return nil
	}
	t.apiKey = ResolveAPIKey(t.opts)
	t.baseURL = ResolveBaseURL(t.opts)
	t.started = true
	return nil
}

// CreateStream creates a streaming chat completion request.
func (t *HTTPTransport) CreateStream(ctx context.Context, req *config.ChatRequest) (<-chan map[string]any, <-chan error) {
	out := make(chan map[string]any, 32)
	errs := make(chan error, 4)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case errs <- fmt.Errorf("panic in CreateStream: %v", r):
				default:
				}
				close(out)
				close(errs)
			}
		}()

		if err := t.Start(ctx); err != nil {
			errs <- err
			close(out)
			close(errs)
			return
		}

		body := t.buildRequestBody(req)
		payload, err := json.Marshal(body)
		if err != nil {
			errs <- fmt.Errorf("marshal request: %w", err)
			close(out)
			close(errs)
			return
		}

		resp, err := t.doRequest(ctx, payload)
		if err != nil {
			errs <- err
			close(out)
			close(errs)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		ct := strings.ToLower(resp.Header.Get("content-type"))
		if !strings.Contains(ct, "text/event-stream") {
			raw, _ := io.ReadAll(resp.Body)
			if strings.Contains(ct, "application/json") {
				var event map[string]any
				if err := json.Unmarshal(raw, &event); err != nil {
					errs <- fmt.Errorf("decode json response: %w", err)
				} else if errVal, ok := event["error"]; ok {
					errs <- fmt.Errorf("lmstudio error: %v", errVal)
				} else {
					select {
					case out <- event:
					case <-ctx.Done():
						errs <- ctx.Err()
					}
				}
				close(out)
				close(errs)
				return
			}
			errs <- fmt.Errorf("expected sse response, got %s: %s", ct, string(raw))
			close(out)
			close(errs)
			return
		}

		util.ParseSSE(ctx, resp.Body, out, errs)
	}()

	return out, errs
}

func (t *HTTPTransport) Close() error { return nil }

func (t *HTTPTransport) doRequest(ctx context.Context, payload []byte) (*http.Response, error) {
	endpoint := "chat/completions"
	u, err := url.JoinPath(strings.TrimSuffix(t.baseURL, "/"), endpoint)
	if err != nil {
		return nil, err
	}

	var lastErr error
	reqCtx, reqSpan := t.obs.StartHTTPSpan(ctx, endpoint)
	defer reqSpan.End()

	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, u, bytes.NewReader(payload))
		if err != nil {
			reqSpan.RecordError(err)
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("User-Agent", "lm-agent-sdk-go/0.1.0")
		if t.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+t.apiKey)
		}

		isRetry := attempt > 0
		reqStarted := time.Now()
		resp, err := t.client.Do(req)
		if err != nil {
			lastErr = err
			reqSpan.RecordError(err)
		} else {
			reqDuration := time.Since(reqStarted).Seconds()
			sc := observability.StatusClassOf(resp.StatusCode)
			t.obs.RecordHTTPRequest(reqCtx, sc, isRetry)
			t.obs.RecordHTTPRequestDuration(reqCtx, reqDuration, sc, isRetry)
			reqSpan.SetAttributes(
				observability.StatusClass(sc),
				observability.Retry(isRetry),
				httpconv.ResponseStatusCode(resp.StatusCode),
			)

			if resp.StatusCode == 429 {
				t.obs.RecordRateLimitEvent(reqCtx)
			}

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return resp, nil
			}
			raw, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			lastErr = &observability.HTTPStatusError{StatusCode: resp.StatusCode, Body: string(raw)}
			reqSpan.RecordError(lastErr)
			if resp.StatusCode < 500 && resp.StatusCode != 429 {
				return nil, lastErr
			}
		}

		if attempt < 2 {
			delay := util.Backoff(attempt)
			reqSpan.AddEvent("retry",
				observability.RetryAttempt(attempt+2),
				observability.RetryDelay(delay),
			)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return nil, lastErr
}

func (t *HTTPTransport) buildRequestBody(req *config.ChatRequest) map[string]any {
	body := map[string]any{
		"model":    strings.TrimSpace(req.Model),
		"messages": cloneMapSlice(req.Messages),
		"stream":   true,
	}

	if req.StreamIncludeUsage != nil && *req.StreamIncludeUsage {
		body["stream_options"] = map[string]any{"include_usage": true}
	}

	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
		if req.ToolChoice != nil {
			body["tool_choice"] = req.ToolChoice
		} else {
			body["tool_choice"] = "auto"
		}
	} else if req.ToolChoice != nil {
		body["tool_choice"] = req.ToolChoice
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
	if req.Logprobs != nil {
		body["logprobs"] = *req.Logprobs
	}
	if req.TopLogprobs != nil {
		body["top_logprobs"] = *req.TopLogprobs
	}
	if req.ParallelToolCalls != nil {
		body["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if len(req.ResponseFormat) > 0 {
		body["response_format"] = normalizeChatResponseFormat(req.ResponseFormat)
	}
	if len(req.Reasoning) > 0 {
		body["reasoning"] = cloneMap(req.Reasoning)
	}
	if req.MaxToolCalls != nil {
		body["max_tool_calls"] = *req.MaxToolCalls
	}
	if req.TTL != nil {
		body["ttl"] = *req.TTL
	}
	if strings.TrimSpace(req.DraftModel) != "" {
		body["draft_model"] = strings.TrimSpace(req.DraftModel)
	}
	if req.User != "" {
		body["user"] = req.User
	}
	if len(req.Extra) > 0 {
		mergeInto(body, req.Extra)
	}

	return body
}

func normalizeChatResponseFormat(format map[string]any) map[string]any {
	if len(format) == 0 {
		return nil
	}
	formatType, _ := format["type"].(string)
	if formatType == "json_schema" {
		if _, ok := format["json_schema"]; ok {
			return cloneMap(format)
		}
		if schema, ok := format["schema"].(map[string]any); ok {
			out := cloneMap(format)
			delete(out, "schema")
			out["json_schema"] = buildJSONSchemaConfig(schema)
			return out
		}
		return cloneMap(format)
	}
	if schema, ok := format["json_schema"].(map[string]any); ok && len(schema) > 0 {
		return map[string]any{
			"type":        "json_schema",
			"json_schema": buildJSONSchemaConfig(schema),
		}
	}
	if schema, ok := format["schema"].(map[string]any); ok && len(schema) > 0 {
		return map[string]any{
			"type":        "json_schema",
			"json_schema": buildJSONSchemaConfig(schema),
		}
	}
	if _, hasProperties := format["properties"]; hasProperties || format["type"] == "object" {
		return map[string]any{
			"type":        "json_schema",
			"json_schema": buildJSONSchemaConfig(format),
		}
	}
	return cloneMap(format)
}

func buildJSONSchemaConfig(schema map[string]any) map[string]any {
	if len(schema) == 0 {
		return map[string]any{"name": "response"}
	}
	if inner, ok := schema["schema"].(map[string]any); ok {
		out := cloneMap(schema)
		out["schema"] = cloneMap(inner)
		if name, ok := out["name"].(string); !ok || strings.TrimSpace(name) == "" {
			out["name"] = "response"
		}
		return out
	}
	return map[string]any{
		"name":   "response",
		"schema": cloneMap(schema),
	}
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMapSlice(in []map[string]any) []map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(in))
	for _, m := range in {
		out = append(out, cloneMap(m))
	}
	return out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func mergeInto(dst map[string]any, extra map[string]any) {
	for k, v := range extra {
		dst[k] = v
	}
}
