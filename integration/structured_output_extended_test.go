//go:build integration

package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestStructuredOutput_RequiredFields(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithOutputFormat(map[string]any{
		"type": "json_schema",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"required": []string{"name", "age"},
		},
	}))

	for msg, err := range lmsdk.Query(ctx, promptText("Return JSON with name set to Alice and age set to 30."), opts...) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}

		result, ok := msg.(*lmsdk.ResultMessage)
		if !ok {
			continue
		}

		parsed, ok := result.StructuredOutput.(map[string]any)
		if !ok {
			t.Fatalf("expected parsed structured output, got %#v", result.StructuredOutput)
		}
		if parsed["name"] == nil {
			t.Fatalf("expected name field, got %#v", parsed)
		}
		if parsed["age"] == nil {
			t.Fatalf("expected age field, got %#v", parsed)
		}

		return
	}

	t.Fatal("expected result message")
}

func TestStructuredOutput_WithEnum(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithOutputFormat(map[string]any{
		"type": "json_schema",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"color": map[string]any{
					"type": "string",
					"enum": []string{"red", "green", "blue"},
				},
				"intensity": map[string]any{
					"type": "string",
					"enum": []string{"low", "medium", "high"},
				},
			},
			"required": []string{"color", "intensity"},
		},
	}))

	for msg, err := range lmsdk.Query(ctx, promptText("Pick a color and intensity. Return as JSON."), opts...) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}

		result, ok := msg.(*lmsdk.ResultMessage)
		if !ok {
			continue
		}

		parsed, ok := result.StructuredOutput.(map[string]any)
		if !ok {
			t.Fatalf("expected parsed structured output, got %#v", result.StructuredOutput)
		}

		color, ok := parsed["color"].(string)
		if !ok {
			t.Fatalf("expected color string, got %#v", parsed["color"])
		}

		validColors := map[string]bool{"red": true, "green": true, "blue": true}
		if !validColors[color] {
			t.Fatalf("expected color to be red/green/blue, got %q", color)
		}

		intensity, ok := parsed["intensity"].(string)
		if !ok {
			t.Fatalf("expected intensity string, got %#v", parsed["intensity"])
		}

		validIntensities := map[string]bool{"low": true, "medium": true, "high": true}
		if !validIntensities[intensity] {
			t.Fatalf("expected intensity to be low/medium/high, got %q", intensity)
		}

		return
	}

	t.Fatal("expected result message")
}

func TestStructuredOutput_NestedSchema(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithOutputFormat(map[string]any{
		"type": "json_schema",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"person": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
						"age":  map[string]any{"type": "integer"},
					},
					"required": []string{"name", "age"},
				},
			},
			"required": []string{"person"},
		},
	}))

	for msg, err := range lmsdk.Query(ctx, promptText("Return JSON with a person object containing name Bob and age 25."), opts...) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}

		result, ok := msg.(*lmsdk.ResultMessage)
		if !ok {
			continue
		}

		parsed, ok := result.StructuredOutput.(map[string]any)
		if !ok {
			t.Fatalf("expected parsed structured output, got %#v", result.StructuredOutput)
		}

		person, ok := parsed["person"].(map[string]any)
		if !ok {
			t.Fatalf("expected person object, got %#v", parsed["person"])
		}
		if person["name"] == nil || person["age"] == nil {
			t.Fatalf("expected name and age in person, got %#v", person)
		}

		return
	}

	t.Fatal("expected result message")
}

func TestStructuredOutput_WithTools(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var executions atomic.Int32
	tool := lmsdk.NewTool("get_data", "Return some data", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, func(_ context.Context, _ map[string]any) (map[string]any, error) {
		executions.Add(1)
		return map[string]any{"value": 42}, nil
	})

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		lmsdk.WithOutputFormat(map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"result": map[string]any{"type": "integer"},
				},
				"required": []string{"result"},
			},
		}),
		// Force tool use: Qwen3 thinking-mode models will happily hallucinate
		// a schema-conforming response instead of calling the tool otherwise.
		lmsdk.WithToolChoice("required"),
		lmsdk.WithSystemPrompt("You MUST call the get_data tool. Do not invent a value. Only the tool knows the real number. After the tool returns, produce JSON matching the schema."),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText("Invoke the get_data tool exactly once (it takes no arguments) and then return the resulting number as JSON.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveMessages(ctx) {
		}
	}()

	if err := waitForCondition(ctx, func() bool { return executions.Load() > 0 }); err != nil {
		t.Fatalf("wait for tool execution: %v", err)
	}
	_ = client.Interrupt(ctx)
	<-done

	if executions.Load() == 0 {
		t.Fatal("expected tool to execute with structured output")
	}
}
