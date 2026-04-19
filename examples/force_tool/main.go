// Package main demonstrates WithForceTool — the LM-Studio-supported way to
// force a specific function since the OpenAI object form of tool_choice is
// rejected. The SDK sets tool_choice="required" and narrows tools[] to just
// the named tool on the wire.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	lookupCalled := false
	lookup := sdk.NewTool(
		"fetch_secret",
		"Fetch a secret integer that cannot be known any other way.",
		map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		func(_ context.Context, _ map[string]any) (map[string]any, error) {
			lookupCalled = true
			return map[string]any{"value": 1337}, nil
		},
	)

	// Dummy second tool so there's something to filter out. Without
	// WithForceTool the model could pick either one.
	decoy := sdk.NewTool(
		"unrelated_helper",
		"Should not be called for this prompt.",
		map[string]any{"type": "object", "properties": map[string]any{}},
		func(_ context.Context, _ map[string]any) (map[string]any, error) {
			return map[string]any{"unused": true}, nil
		},
	)

	for msg, err := range sdk.Query(ctx,
		sdk.Text("Retrieve the secret value and tell me only the number."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSDKTools(lookup, decoy),
		sdk.WithForceTool("mcp__sdk__fetch_secret"),
		sdk.WithTemperature(0.1),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		if result, ok := msg.(*sdk.ResultMessage); ok && result.Result != nil {
			text := strings.TrimSpace(*result.Result)
			fmt.Printf("Final answer: %s\n", text)
		}
	}
	if lookupCalled {
		fmt.Println("fetch_secret was invoked (as forced).")
	} else {
		fmt.Println("fetch_secret was NOT invoked (unexpected).")
	}
}
