//go:build integration

package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestHooks(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	tool := lmsdk.NewTool("echo", "Echo text", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"echo": input["text"]}, nil
	})

	var preCount atomic.Int32
	var postCount atomic.Int32
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		lmsdk.WithToolChoice("auto"),
		lmsdk.WithHooks(map[lmsdk.HookEvent][]*lmsdk.HookMatcher{
			lmsdk.HookEventPreToolUse: {{
				Hooks: []lmsdk.HookCallback{
					func(context.Context, lmsdk.HookInput, *string, *lmsdk.HookContext) (lmsdk.HookJSONOutput, error) {
						preCount.Add(1)
						return &lmsdk.SyncHookJSONOutput{}, nil
					},
				},
			}},
			lmsdk.HookEventPostToolUse: {{
				Hooks: []lmsdk.HookCallback{
					func(context.Context, lmsdk.HookInput, *string, *lmsdk.HookContext) (lmsdk.HookJSONOutput, error) {
						postCount.Add(1)
						return &lmsdk.SyncHookJSONOutput{}, nil
					},
				},
			}},
		}),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText(`Call the echo tool with {"text":"hooked"}.`)); err != nil {
		t.Fatalf("query: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveMessages(ctx) {
		}
	}()

	if err := waitForCondition(ctx, func() bool { return postCount.Load() > 0 }); err != nil {
		t.Fatalf("wait for hook execution: %v", err)
	}
	_ = client.Interrupt(ctx)
	<-done

	if preCount.Load() == 0 || postCount.Load() == 0 {
		t.Fatalf("expected pre/post hooks to fire, got pre=%d post=%d", preCount.Load(), postCount.Load())
	}
}
