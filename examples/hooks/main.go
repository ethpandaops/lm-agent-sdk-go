package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	tool := sdk.NewTool("echo", "Echo text", map[string]any{
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

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for msg, err := range sdk.Query(ctx,
		sdk.Text(`You must call the echo tool exactly once with {"text":"hooked"}. After the tool returns, answer with exactly: done.`),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSDKTools(tool),
		sdk.WithHooks(map[sdk.HookEvent][]*sdk.HookMatcher{
			sdk.HookEventPreToolUse: {{
				Hooks: []sdk.HookCallback{
					func(context.Context, sdk.HookInput, *string, *sdk.HookContext) (sdk.HookJSONOutput, error) {
						preCount.Add(1)
						return &sdk.SyncHookJSONOutput{}, nil
					},
				},
			}},
			sdk.HookEventPostToolUse: {{
				Hooks: []sdk.HookCallback{
					func(context.Context, sdk.HookInput, *string, *sdk.HookContext) (sdk.HookJSONOutput, error) {
						postCount.Add(1)
						return &sdk.SyncHookJSONOutput{}, nil
					},
				},
			}},
		}),
		sdk.WithMaxTurns(4),
		sdk.WithTemperature(0),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}

	fmt.Printf("pre-tool hooks: %d\n", preCount.Load())
	fmt.Printf("post-tool hooks: %d\n", postCount.Load())
}
