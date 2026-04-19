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

// LoadConfig is the parameter set accepted by POST /api/v1/models/load. All
// fields are optional; server defaults apply when a pointer field is nil or a
// string/slice field is empty. Field names match LM Studio's REST schema
// verbatim (snake_case) so unknown-key errors don't leak SDK naming choices.
type LoadConfig struct {
	// ContextLength in tokens for prompt + response. Zero = server default.
	ContextLength int `json:"context_length,omitempty"`
	// EvalBatchSize controls llama.cpp / MLX prompt eval batch size.
	EvalBatchSize int `json:"eval_batch_size,omitempty"`
	// FlashAttention toggles flash attention. LM Studio's default often ends
	// up off even when "auto" — set explicitly for best throughput.
	FlashAttention *bool `json:"flash_attention,omitempty"`
	// OffloadKVCacheToGPU requests GPU residency for the KV cache.
	OffloadKVCacheToGPU *bool `json:"offload_kv_cache_to_gpu,omitempty"`
	// TTLSeconds sets the idle TTL before the JIT loader evicts the model.
	TTLSeconds int `json:"ttl_seconds,omitempty"`
	// Seed overrides the RNG seed at load time.
	Seed *int64 `json:"seed,omitempty"`
	// NumExperts overrides MoE active-expert count.
	NumExperts int `json:"num_experts,omitempty"`
	// KeepInMemory requests mlock-like behavior.
	KeepInMemory *bool `json:"keep_in_memory,omitempty"`
	// UseFp16ForKVCache stores KV cache in FP16 instead of FP32.
	UseFp16ForKVCache *bool `json:"use_fp16_for_kv_cache,omitempty"`
	// TryMmap requests memory-mapped GGUF load.
	TryMmap *bool `json:"try_mmap,omitempty"`
}

// LoadRequest is the body of POST /api/v1/models/load.
type LoadRequest struct {
	Model string `json:"model"`
	LoadConfig
}

// LoadResponse describes a successfully loaded instance.
type LoadResponse struct {
	InstanceID string          `json:"instance_id"`
	Model      string          `json:"model"`
	Config     json.RawMessage `json:"config,omitempty"`
}

// UnloadRequest is the body of POST /api/v1/models/unload.
type UnloadRequest struct {
	InstanceID string `json:"instance_id"`
}

// DownloadRequest is the body of POST /api/v1/models/download.
type DownloadRequest struct {
	// Model is the LM Studio catalog key or Hugging Face repo reference.
	Model string `json:"model"`
	// Variant is an optional quantization variant to fetch.
	Variant string `json:"variant,omitempty"`
}

// DownloadResponse is returned immediately from a download request; poll the
// status endpoint with the returned job ID for progress.
type DownloadResponse struct {
	JobID string `json:"job_id"`
}

// DownloadStatus reports progress for an in-flight download job.
type DownloadStatus struct {
	JobID           string  `json:"job_id"`
	Model           string  `json:"model,omitempty"`
	State           string  `json:"state"`
	BytesDownloaded int64   `json:"bytes_downloaded,omitempty"`
	BytesTotal      int64   `json:"bytes_total,omitempty"`
	Progress        float64 `json:"progress,omitempty"`
	Error           string  `json:"error,omitempty"`
}

// LoadModel calls POST /api/v1/models/load and returns the loaded instance
// reference. Returns an error with the server-reported reason on failure
// (including unrecognized fields in LoadConfig, out-of-memory, or unsupported
// model format).
func LoadModel(ctx context.Context, opts *config.Options, req LoadRequest) (*LoadResponse, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("LoadModel: model is required")
	}
	var resp LoadResponse
	if err := postJSON(ctx, opts, "models/load", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// UnloadModel calls POST /api/v1/models/unload. `instanceID` is the ID
// returned by LoadModel or exposed in GET /api/v1/models → loaded_instances.
func UnloadModel(ctx context.Context, opts *config.Options, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return fmt.Errorf("UnloadModel: instance ID is required")
	}
	var ack struct {
		OK bool `json:"ok"`
	}
	return postJSON(ctx, opts, "models/unload", UnloadRequest{InstanceID: instanceID}, &ack)
}

// DownloadModel calls POST /api/v1/models/download to start an async fetch
// from the LM Studio catalog or Hugging Face. Returns the job ID for status
// polling via DownloadStatusFor.
func DownloadModel(ctx context.Context, opts *config.Options, req DownloadRequest) (*DownloadResponse, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("DownloadModel: model is required")
	}
	var resp DownloadResponse
	if err := postJSON(ctx, opts, "models/download", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DownloadStatusFor polls GET /api/v1/models/download/status?job_id=...
// for an in-flight download job started via DownloadModel.
func DownloadStatusFor(ctx context.Context, opts *config.Options, jobID string) (*DownloadStatus, error) {
	if strings.TrimSpace(jobID) == "" {
		return nil, fmt.Errorf("DownloadStatusFor: job ID is required")
	}
	u, err := nativeURL(opts, "models/download/status")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("job_id", jobID)
	u = u + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	applyHeaders(req, opts, false)

	client := nativeHTTPClient(opts)
	httpResp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = httpResp.Body.Close() }()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, fmt.Errorf("lmstudio download status status=%d body=%s", httpResp.StatusCode, string(body))
	}
	var out DownloadStatus
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode download status: %w", err)
	}
	return &out, nil
}

func postJSON(ctx context.Context, opts *config.Options, path string, reqBody any, out any) error {
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal %s request: %w", path, err)
	}
	u, err := nativeURL(opts, path)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	applyHeaders(httpReq, opts, true)

	client := nativeHTTPClient(opts)
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s response: %w", path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeLMStudioError(body, resp.StatusCode, path)
	}
	if len(body) == 0 || out == nil {
		return nil
	}
	// LM Studio sometimes returns 200 with {"error": ...}.
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err == nil {
		if errVal, ok := probe["error"]; ok {
			return fmt.Errorf("lmstudio %s error: %v", path, errVal)
		}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}

func nativeURL(opts *config.Options, path string) (string, error) {
	if opts != nil {
		opts.ApplyDefaults()
	} else {
		opts = &config.Options{}
		opts.ApplyDefaults()
	}
	base := deriveNativeV1Base(ResolveBaseURL(opts))
	return url.JoinPath(strings.TrimSuffix(base, "/"), path)
}

// deriveNativeV1Base converts an OpenAI-compat base URL (`…/v1`) into its
// `…/api/v1` sibling used by the model lifecycle endpoints.
func deriveNativeV1Base(base string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return strings.TrimSuffix(base, "/v1") + "/api/v1"
	}
	if strings.HasSuffix(base, "/api/v1") {
		return base
	}
	return base + "/api/v1"
}

func applyHeaders(req *http.Request, opts *config.Options, writeBody bool) {
	if writeBody {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lm-agent-sdk-go/0.1.0")
	if apiKey := ResolveAPIKey(opts); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
}

func decodeLMStudioError(body []byte, status int, path string) error {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
			Param   string `json:"param"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err == nil && env.Error.Message != "" {
		if env.Error.Code != "" {
			return fmt.Errorf("lmstudio %s: %s (%s)", path, env.Error.Message, env.Error.Code)
		}
		return fmt.Errorf("lmstudio %s: %s", path, env.Error.Message)
	}
	return fmt.Errorf("lmstudio %s status=%d body=%s", path, status, string(body))
}

// nativeHTTPClientLong is the client used for model lifecycle calls which can
// take a while (especially LoadModel on big models). Callers should still pass
// a reasonable ctx deadline.
var _ = time.Minute // keep import
