package lmsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestListModelsResponse_FallbackReturnsRealCatalogError(t *testing.T) {
	server := newModelTestServer(t, func(path string) (int, map[string]any) {
		switch path {
		case "/api/v1/models":
			return 500, map[string]any{"error": map[string]any{"message": "catalog down"}}
		default:
			return 404, map[string]any{"error": map[string]any{"message": "missing"}}
		}
	})
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ListModelsResponse(ctx, WithBaseURL(server.URL+"/api/v1"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "catalog down") {
		t.Fatalf("expected real fallback error, got %v", err)
	}
	if strings.Contains(err.Error(), "missing LM_API_KEY") {
		t.Fatalf("unexpected auth error leak: %v", err)
	}
}

func newModelTestServer(t *testing.T, handler func(path string) (int, map[string]any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, payload := handler(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
}

func TestModelInfoCapabilityHelpers(t *testing.T) {
	info := ModelInfo{
		ID:            "vision-reasoner",
		ContextLength: 128000,
		Architecture:  &ModelArchitecture{Modality: "multimodal-vision"},
		Pricing: &ModelPricing{
			Prompt:          "0",
			Completion:      "0.000002",
			WebSearch:       "0.001",
			InputCacheRead:  "0.0000001",
			InputCacheWrite: "0.0000002",
		},
		TopProvider: &ModelTopProvider{
			ContextLength:       200000,
			MaxCompletionTokens: 16000,
		},
		PerRequestLimits: &ModelPerRequestLimits{
			CompletionTokens: 8000,
			Images:           1,
		},
		Endpoints: []ModelEndpoint{
			{Name: "chat", ContextLength: 100000, MaxCompletionTokens: 4096},
			{Name: "responses", ContextLength: 200000, MaxCompletionTokens: 12000},
		},
		SupportedParameters: ModelSupportedParameters{"tools", "tool_choice", "response_format", "reasoning", "web_search"},
		IsReasoning:         true,
	}

	if !info.SupportsToolCalling() {
		t.Fatal("expected tool calling support")
	}
	if !info.SupportsStructuredOutput() {
		t.Fatal("expected structured output support")
	}
	if !info.SupportsReasoning() {
		t.Fatal("expected reasoning support")
	}
	if !info.SupportsImageInput() {
		t.Fatal("expected image input support")
	}
	if !info.SupportsImageOutput() {
		t.Fatal("expected image output support")
	}
	if !info.SupportsWebSearch() {
		t.Fatal("expected web search support")
	}
	if !info.SupportsPromptCaching() {
		t.Fatal("expected prompt caching support")
	}
	if got := info.MaxContextLength(); got != 200000 {
		t.Fatalf("unexpected max context length: %d", got)
	}
	if got := info.MaxCompletionTokens(); got != 16000 {
		t.Fatalf("unexpected max completion tokens: %d", got)
	}
	if got, ok := info.PromptPriceUSD(); !ok || got != 0 {
		t.Fatalf("unexpected prompt price: %v %v", got, ok)
	}
	if got, ok := info.CompletionPriceUSD(); !ok || got != 0.000002 {
		t.Fatalf("unexpected completion price: %v %v", got, ok)
	}
	if got, ok := info.RequestPriceUSD(); ok || got != 0 {
		t.Fatalf("unexpected request price: %v %v", got, ok)
	}
	if info.CostTier() != "paid" {
		t.Fatalf("unexpected cost tier: %s", info.CostTier())
	}
}

func TestModelInfoToModelUsesDerivedFlags(t *testing.T) {
	info := ModelInfo{
		ID:            "free-model",
		Name:          "Free Model",
		Description:   "desc",
		ContextLength: 64000,
		Pricing: &ModelPricing{
			Prompt:     "0",
			Completion: "0",
		},
		IsReasoning: true,
	}

	got := info.ToModel()
	want := Model{
		ID:          "free-model",
		Name:        "Free Model",
		Description: "desc",
		Context:     64000,
		IsFree:      true,
		IsReasoning: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected model projection: %#v", got)
	}
}
