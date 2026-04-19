//go:build integration

package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestHooks_BlockTool(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var toolExecuted atomic.Bool
	tool := lmsdk.NewTool("blocked_echo", "Echo text", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}, func(_ context.Context, _ map[string]any) (map[string]any, error) {
		toolExecuted.Store(true)
		return map[string]any{"echo": "should not reach"}, nil
	})

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		lmsdk.WithToolChoice("auto"),
		lmsdk.WithHooks(map[lmsdk.HookEvent][]*lmsdk.HookMatcher{
			lmsdk.HookEventPreToolUse: {{
				Hooks: []lmsdk.HookCallback{
					func(_ context.Context, _ lmsdk.HookInput, _ *string, _ *lmsdk.HookContext) (lmsdk.HookJSONOutput, error) {
						return &lmsdk.SyncHookJSONOutput{
							Continue: new(bool), // false blocks tool execution
						}, nil
					},
				},
			}},
		}),
	)

	// The tool should be blocked, so we expect it to complete (possibly with error)
	// but the tool handler should never execute.
	for _, err := range lmsdk.Query(ctx, promptText(`Call blocked_echo with {"text":"test"}.`), opts...) {
		if err != nil {
			// Blocking a tool may produce an error, which is expected.
			break
		}
	}

	if toolExecuted.Load() {
		t.Fatal("expected tool to be blocked by PreToolUse hook")
	}
}

func TestHooks_PostToolUseWithContext(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var postInput atomic.Value
	tool := lmsdk.NewTool("context_echo", "Echo text", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"echo": input["text"]}, nil
	})

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		lmsdk.WithToolChoice("auto"),
		lmsdk.WithHooks(map[lmsdk.HookEvent][]*lmsdk.HookMatcher{
			lmsdk.HookEventPostToolUse: {{
				Hooks: []lmsdk.HookCallback{
					func(_ context.Context, input lmsdk.HookInput, _ *string, _ *lmsdk.HookContext) (lmsdk.HookJSONOutput, error) {
						postInput.Store(input)
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

	if err := client.Query(ctx, promptText(`Call context_echo with {"text":"hooked"}.`)); err != nil {
		t.Fatalf("query: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveMessages(ctx) {
		}
	}()

	if err := waitForCondition(ctx, func() bool { return postInput.Load() != nil }); err != nil {
		t.Fatalf("wait for post hook: %v", err)
	}
	_ = client.Interrupt(ctx)
	<-done

	stored := postInput.Load()
	if stored == nil {
		t.Fatal("expected PostToolUse hook input to be captured")
	}

	postHookInput, ok := stored.(*lmsdk.PostToolUseHookInput)
	if !ok {
		t.Fatalf("expected PostToolUseHookInput, got %T", stored)
	}
	if postHookInput.ToolName == "" {
		t.Fatal("expected tool name in PostToolUse hook input")
	}
}
