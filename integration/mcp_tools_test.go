//go:build integration

package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestSDKTools(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var executions atomic.Int32
	tool := lmsdk.NewTool("echo", "Echo text", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		executions.Add(1)
		return map[string]any{"echo": input["text"]}, nil
	})

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		// LM Studio only supports string tool_choice values.
		lmsdk.WithToolChoice("auto"),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText(`Call the echo tool with {"text":"integration-echo"}.`)); err != nil {
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
		t.Fatal("expected SDK tool execution")
	}
}
