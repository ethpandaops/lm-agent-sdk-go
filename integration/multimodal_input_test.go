//go:build integration

package integration_test

import (
	"strings"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

// 8x8 red square PNG — small but large enough for providers to decode.
const tinyPNGDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAgAAAAICAIAAABLbSncAAAAEklEQVR4nGP4z8CAFWEXHbQSACj/P8Fu7N9hAAAAAElFTkSuQmCC"

func TestMultimodalImageInputQuery(t *testing.T) {
	ctx, cancel := integrationContextForBaseURL(t, integrationVisionBaseURL())
	defer cancel()

	content := lmsdk.Blocks(
		lmsdk.TextInput("Describe the provided placeholder image in under ten words."),
		lmsdk.ImageInput(tinyPNGDataURL),
	)

	var result *lmsdk.ResultMessage
	opts := append(integrationVisionOptions(t),
		lmsdk.WithMaxTurns(2),
		lmsdk.WithTemperature(0),
	)
	model := integrationVisionModel(t)
	for msg, err := range lmsdk.Query(ctx, content, opts...) {
		if err != nil {
			// Vision support is provider-dependent; skip on provider errors.
			t.Skipf("provider error (model %s may not support image input): %v", model, err)
		}
		if r, ok := msg.(*lmsdk.ResultMessage); ok {
			result = r
		}
	}
	if result == nil || result.Result == nil {
		t.Fatal("expected result from multimodal input query")
	}
	if strings.TrimSpace(*result.Result) == "" {
		t.Fatalf("expected non-empty multimodal result, got %+v", result)
	}
}
