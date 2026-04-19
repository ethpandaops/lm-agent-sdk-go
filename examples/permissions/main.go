package main

import (
	"context"
	"errors"
	"fmt"
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

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for _, err := range sdk.Query(ctx,
		sdk.Text(`Call the echo tool with {"text":"blocked"}.`),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSDKTools(tool),
		sdk.WithCanUseTool(func(_ context.Context, name string, _ map[string]any, _ *sdk.ToolPermissionContext) (sdk.PermissionResult, error) {
			return &sdk.PermissionResultDeny{Behavior: "deny", Message: "example policy denied " + name}, nil
		}),
		sdk.WithMaxTurns(4),
		sdk.WithTemperature(0),
	) {
		if err == nil {
			continue
		}
		var denied *sdk.ToolPermissionDeniedError
		if errors.As(err, &denied) {
			fmt.Printf("Permission denied as expected: %v\n", denied)
			return
		}
		fmt.Printf("query error: %v\n", err)
		return
	}

	fmt.Println("Expected a permission denial but the query completed.")
}
