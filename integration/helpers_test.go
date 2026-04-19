//go:build integration

package integration_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

const (
	defaultIntegrationBaseURL = "http://127.0.0.1:1234/v1"
	defaultIntegrationModel   = "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ"
)

func integrationContext(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return integrationContextForBaseURL(t, integrationBaseURL())
}

func integrationContextForBaseURL(t *testing.T, baseURL string) (context.Context, context.CancelFunc) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	req, err := http.NewRequest(http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		t.Skipf("integration base url invalid: %v", err)
	}
	if apiKey := integrationAPIKey(); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("vLLM integration server unavailable at %s: %v", baseURL, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		t.Skipf("vLLM integration server returned status %d from %s/models", resp.StatusCode, baseURL)
	}
	return context.WithTimeout(context.Background(), 90*time.Second)
}

func integrationOptions() []lmsdk.Option {
	return integrationOptionsFor(integrationBaseURL(), integrationModel())
}

func integrationOptionsFor(baseURL, model string) []lmsdk.Option {
	opts := []lmsdk.Option{
		lmsdk.WithBaseURL(baseURL),
		lmsdk.WithModel(model),
		lmsdk.WithMaxTurns(6),
		lmsdk.WithTemperature(0),
	}
	if apiKey := integrationAPIKey(); apiKey != "" {
		opts = append(opts, lmsdk.WithAPIKey(apiKey))
	}
	return opts
}

func integrationBaseURL() string {
	if baseURL := os.Getenv("LM_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return defaultIntegrationBaseURL
}

func integrationModel() string {
	if model := os.Getenv("LM_MODEL"); model != "" {
		return model
	}
	return defaultIntegrationModel
}

func integrationAPIKey() string {
	if apiKey := os.Getenv("LM_API_KEY"); apiKey != "" {
		return apiKey
	}
	return ""
}

func promptText(text string) lmsdk.UserMessageContent {
	return lmsdk.Text(text)
}

func integrationImageModel(t *testing.T) string {
	t.Helper()
	model := os.Getenv("LM_IMAGE_MODEL")
	if model == "" {
		t.Skip("LM_IMAGE_MODEL is required for image-output integration tests")
	}
	return model
}

func integrationImageBaseURL() string {
	if baseURL := os.Getenv("LM_IMAGE_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return integrationBaseURL()
}

func integrationImageOptions(t *testing.T) []lmsdk.Option {
	t.Helper()
	return integrationOptionsFor(integrationImageBaseURL(), integrationImageModel(t))
}

func integrationVisionModel(t *testing.T) string {
	t.Helper()
	if model := os.Getenv("VLLM_VISION_MODEL"); model != "" {
		return model
	}
	if model := os.Getenv("VLLM_IMAGE_MODEL"); model != "" {
		return model
	}
	return defaultIntegrationModel
}

func integrationVisionBaseURL() string {
	if baseURL := os.Getenv("VLLM_VISION_BASE_URL"); baseURL != "" {
		return baseURL
	}
	if baseURL := os.Getenv("VLLM_IMAGE_BASE_URL"); baseURL != "" {
		return baseURL
	}
	return integrationBaseURL()
}

func integrationVisionOptions(t *testing.T) []lmsdk.Option {
	t.Helper()
	return integrationOptionsFor(integrationVisionBaseURL(), integrationVisionModel(t))
}

func collectResult(
	t *testing.T,
	iter func(func(lmsdk.Message, error) bool),
) *lmsdk.ResultMessage {
	t.Helper()
	var result *lmsdk.ResultMessage
	iter(func(msg lmsdk.Message, err error) bool {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if rm, ok := msg.(*lmsdk.ResultMessage); ok {
			result = rm
		}
		return true
	})
	if result == nil {
		t.Fatal("expected result message")
	}
	return result
}

func waitForSession(
	ctx context.Context,
	sessionID string,
	opts ...lmsdk.Option,
) (*lmsdk.SessionStat, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		stat, err := lmsdk.StatSession(ctx, sessionID, opts...)
		if err == nil {
			return stat, nil
		}
		if !errors.Is(err, lmsdk.ErrSessionNotFound) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForCondition(ctx context.Context, check func() bool) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		if check() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
