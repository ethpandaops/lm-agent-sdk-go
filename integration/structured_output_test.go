//go:build integration

package integration_test

import (
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestStructuredOutput(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithOutputFormat(map[string]any{
		"type": "json_schema",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]any{"type": "string"},
			},
			"required": []string{"answer"},
		},
	}))

	for msg, err := range lmsdk.Query(ctx, promptText("Return JSON with answer set to ok."), opts...) {
		if err != nil {
			t.Fatalf("structured output query error: %v", err)
		}
		result, ok := msg.(*lmsdk.ResultMessage)
		if !ok {
			continue
		}
		parsed, ok := result.StructuredOutput.(map[string]any)
		if !ok {
			t.Fatalf("expected parsed structured output, got %#v", result.StructuredOutput)
		}
		if parsed["answer"] == nil {
			t.Fatalf("expected answer field, got %#v", parsed)
		}
		return
	}

	t.Fatal("expected result message")
}
