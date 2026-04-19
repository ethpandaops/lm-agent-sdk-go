//go:build integration

package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestWithForceTool_CallsOnlyNamedTool(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var targetCalls, decoyCalls atomic.Int32

	target := lmsdk.NewTool("target_tool", "The one that must be called",
		map[string]any{"type": "object", "properties": map[string]any{}},
		func(_ context.Context, _ map[string]any) (map[string]any, error) {
			targetCalls.Add(1)
			return map[string]any{"answer": "target-invoked"}, nil
		},
	)
	decoy := lmsdk.NewTool("decoy_tool", "Should never be called in this test",
		map[string]any{"type": "object", "properties": map[string]any{}},
		func(_ context.Context, _ map[string]any) (map[string]any, error) {
			decoyCalls.Add(1)
			return map[string]any{"wrong": true}, nil
		},
	)

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(target, decoy),
		lmsdk.WithForceTool("mcp__sdk__target_tool"),
		lmsdk.WithSystemPrompt("You MUST call the available tool. Do not invent a value."),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText("Invoke the available tool once.")); err != nil {
		t.Fatalf("query: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveMessages(ctx) {
		}
	}()
	if err := waitForCondition(ctx, func() bool { return targetCalls.Load() > 0 }); err != nil {
		t.Fatalf("wait for target tool: %v (target=%d decoy=%d)", err, targetCalls.Load(), decoyCalls.Load())
	}
	_ = client.Interrupt(ctx)
	<-done

	if decoyCalls.Load() != 0 {
		t.Errorf("decoy was called %d times — WithForceTool should have filtered it out", decoyCalls.Load())
	}
}
