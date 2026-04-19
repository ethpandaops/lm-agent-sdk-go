package model

import (
	"strconv"
	"strings"
)

// Architecture describes the underlying model family where available.
type Architecture struct {
	Modality     string `json:"modality,omitempty"`
	Tokenizer    string `json:"tokenizer,omitempty"`
	InstructType string `json:"instruct_type,omitempty"`
}

// Pricing contains VLLM pricing fields as returned by the API.
type Pricing struct {
	Prompt            string `json:"prompt,omitempty"`
	Completion        string `json:"completion,omitempty"`
	Request           string `json:"request,omitempty"`
	Image             string `json:"image,omitempty"`
	InputCacheRead    string `json:"input_cache_read,omitempty"`
	InputCacheWrite   string `json:"input_cache_write,omitempty"`
	WebSearch         string `json:"web_search,omitempty"`
	InternalReasoning string `json:"internal_reasoning,omitempty"`
}

// TopProvider describes provider-side limits and moderation behavior.
type TopProvider struct {
	ContextLength       int  `json:"context_length,omitempty"`
	MaxCompletionTokens int  `json:"max_completion_tokens,omitempty"`
	IsModerated         bool `json:"is_moderated,omitempty"`
}

// PerRequestLimits captures model-side request limits where available.
type PerRequestLimits struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	Images           int `json:"images,omitempty"`
}

// SupportedParameters records provider-supported request parameters.
type SupportedParameters []string

// Endpoint identifies an endpoint a model supports.
type Endpoint struct {
	Name                string `json:"name,omitempty"`
	ContextLength       int    `json:"context_length,omitempty"`
	MaxCompletionTokens int    `json:"max_completion_tokens,omitempty"`
}

// Info describes a model returned by model discovery.
type Info struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name,omitempty"`
	CanonicalSlug       string              `json:"canonical_slug,omitempty"`
	Description         string              `json:"description,omitempty"`
	ContextLength       int                 `json:"context_length,omitempty"`
	Architecture        *Architecture       `json:"architecture,omitempty"`
	Pricing             *Pricing            `json:"pricing,omitempty"`
	TopProvider         *TopProvider        `json:"top_provider,omitempty"`
	PerRequestLimits    *PerRequestLimits   `json:"per_request_limits,omitempty"`
	SupportedParameters SupportedParameters `json:"supported_parameters,omitempty"`
	Endpoints           []Endpoint          `json:"endpoints,omitempty"`
	DefaultEndpoint     string              `json:"default_endpoint,omitempty"`
	IsModerated         bool                `json:"is_moderated,omitempty"`
	IsReasoning         bool                `json:"is_reasoning,omitempty"`
	IsFree              bool                `json:"is_free,omitempty"`
	Publisher           string              `json:"publisher,omitempty"`
	Quantization        string              `json:"quantization,omitempty"`
	CompatibilityType   string              `json:"compatibility_type,omitempty"`
	State               string              `json:"state,omitempty"`
	Capabilities        []string            `json:"capabilities,omitempty"`
	Metadata            map[string]any      `json:"metadata,omitempty"`
}

// Model is a stable provider-neutral wrapper for a discovered model.
type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Context     int    `json:"context,omitempty"`
	IsFree      bool   `json:"is_free,omitempty"`
	IsReasoning bool   `json:"is_reasoning,omitempty"`
}

// ListResponse contains the full model discovery payload.
type ListResponse struct {
	Object        string `json:"object,omitempty"`
	Source        string `json:"source,omitempty"`
	Models        []Info `json:"models"`
	RawData       []Info `json:"data,omitempty"`
	Total         int    `json:"total"`
	Authenticated bool   `json:"authenticated"`
	Endpoint      string `json:"endpoint"`
}

// SupportsToolCalling reports whether the model exposes tool-related parameters.
func (i Info) SupportsToolCalling() bool {
	return i.hasParameter("tools", "tool_choice", "parallel_tool_calls")
}

// SupportsStructuredOutput reports whether JSON-schema style output controls are exposed.
func (i Info) SupportsStructuredOutput() bool {
	return i.hasParameter("response_format", "json_schema", "structured_outputs")
}

// SupportsReasoning reports whether the model exposes explicit reasoning support.
func (i Info) SupportsReasoning() bool {
	return i.IsReasoning || i.hasParameter("reasoning")
}

// SupportsImageInput reports whether the model appears to accept image or multimodal input.
func (i Info) SupportsImageInput() bool {
	if i.Architecture != nil {
		modality := strings.ToLower(i.Architecture.Modality)
		if strings.Contains(modality, "image") || strings.Contains(modality, "vision") || strings.Contains(modality, "multimodal") {
			return true
		}
	}
	return i.hasMetadataFlag("image_input", "supports_image_input", "vision", "multimodal")
}

// SupportsImageOutput reports whether the model appears to support generated image output.
func (i Info) SupportsImageOutput() bool {
	if i.PerRequestLimits != nil && i.PerRequestLimits.Images > 0 {
		return true
	}
	if i.hasParameter("modalities") {
		return true
	}
	return i.hasMetadataModality("output_modalities", "image") ||
		i.hasMetadataFlag("image_output", "supports_image_output")
}

// SupportsWebSearch reports whether the model metadata exposes web-search controls.
func (i Info) SupportsWebSearch() bool {
	return i.hasParameter("web_search") || (i.Pricing != nil && strings.TrimSpace(i.Pricing.WebSearch) != "")
}

// SupportsPromptCaching reports whether cache read/write pricing is exposed.
func (i Info) SupportsPromptCaching() bool {
	if i.Pricing == nil {
		return false
	}
	return strings.TrimSpace(i.Pricing.InputCacheRead) != "" || strings.TrimSpace(i.Pricing.InputCacheWrite) != ""
}

// MaxContextLength returns the best available context-length value across model metadata.
func (i Info) MaxContextLength() int {
	best := i.ContextLength
	if i.TopProvider != nil && i.TopProvider.ContextLength > best {
		best = i.TopProvider.ContextLength
	}
	for _, ep := range i.Endpoints {
		if ep.ContextLength > best {
			best = ep.ContextLength
		}
	}
	return best
}

// MaxCompletionTokens returns the best available completion-token ceiling across model metadata.
func (i Info) MaxCompletionTokens() int {
	best := 0
	if i.TopProvider != nil && i.TopProvider.MaxCompletionTokens > best {
		best = i.TopProvider.MaxCompletionTokens
	}
	if i.PerRequestLimits != nil && i.PerRequestLimits.CompletionTokens > best {
		best = i.PerRequestLimits.CompletionTokens
	}
	for _, ep := range i.Endpoints {
		if ep.MaxCompletionTokens > best {
			best = ep.MaxCompletionTokens
		}
	}
	return best
}

// PromptPriceUSD returns the parsed prompt price when present.
func (i Info) PromptPriceUSD() (float64, bool) {
	return parsePricingValue(i.Pricing, func(p *Pricing) string { return p.Prompt })
}

// CompletionPriceUSD returns the parsed completion price when present.
func (i Info) CompletionPriceUSD() (float64, bool) {
	return parsePricingValue(i.Pricing, func(p *Pricing) string { return p.Completion })
}

// RequestPriceUSD returns the parsed per-request price when present.
func (i Info) RequestPriceUSD() (float64, bool) {
	return parsePricingValue(i.Pricing, func(p *Pricing) string { return p.Request })
}

// ImagePriceUSD returns the parsed image price when present.
func (i Info) ImagePriceUSD() (float64, bool) {
	return parsePricingValue(i.Pricing, func(p *Pricing) string { return p.Image })
}

// CostTier reports a coarse pricing tier derived from the discovery payload.
func (i Info) CostTier() string {
	if i.IsFree || isZeroPricing(i.Pricing) {
		return "free"
	}
	return "paid"
}

// ToModel returns a stable provider-neutral projection of the model info.
func (i Info) ToModel() Model {
	return Model{
		ID:          i.ID,
		Name:        i.Name,
		Description: i.Description,
		Context:     i.ContextLength,
		IsFree:      i.IsFree || isZeroPricing(i.Pricing),
		IsReasoning: i.IsReasoning,
	}
}

func isZeroPricing(p *Pricing) bool {
	if p == nil {
		return false
	}
	for _, raw := range []string{p.Prompt, p.Completion, p.Request, p.Image} {
		if raw == "" {
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v != 0 {
			return false
		}
	}
	return p.Prompt != "" || p.Completion != "" || p.Request != "" || p.Image != ""
}

func (i Info) hasParameter(names ...string) bool {
	if len(i.SupportedParameters) == 0 {
		return false
	}
	for _, supported := range i.SupportedParameters {
		supported = strings.ToLower(strings.TrimSpace(supported))
		for _, name := range names {
			if supported == strings.ToLower(name) {
				return true
			}
		}
	}
	return false
}

func (i Info) hasMetadataFlag(keys ...string) bool {
	if len(i.Metadata) == 0 {
		return false
	}
	for _, key := range keys {
		raw, ok := i.Metadata[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case bool:
			if v {
				return true
			}
		case string:
			lower := strings.ToLower(strings.TrimSpace(v))
			if lower == "true" || lower == "yes" || lower == "supported" {
				return true
			}
		}
	}
	return false
}

func (i Info) hasMetadataModality(key string, want string) bool {
	if len(i.Metadata) == 0 {
		return false
	}
	raw, ok := i.Metadata[key]
	if !ok {
		return false
	}
	want = strings.ToLower(strings.TrimSpace(want))
	switch v := raw.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(v)) == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.ToLower(strings.TrimSpace(s)) == want {
				return true
			}
		}
	case []string:
		for _, item := range v {
			if strings.ToLower(strings.TrimSpace(item)) == want {
				return true
			}
		}
	}
	return false
}

func parsePricingValue(p *Pricing, get func(*Pricing) string) (float64, bool) {
	if p == nil {
		return 0, false
	}
	raw := strings.TrimSpace(get(p))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
