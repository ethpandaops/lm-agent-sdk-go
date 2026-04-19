//go:build integration

package integration_test

import (
	"context"
	"strings"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestPermissionCallback(t *testing.T) {
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

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		lmsdk.WithToolChoice("auto"),
		lmsdk.WithCanUseTool(func(_ context.Context, name string, input map[string]any, _ *lmsdk.ToolPermissionContext) (lmsdk.PermissionResult, error) {
			if name != "mcp__sdk__echo" {
				t.Fatalf("unexpected tool: %s", name)
			}
			return &lmsdk.PermissionResultDeny{Behavior: "deny", Message: "integration denied"}, nil
		}),
	)

	var gotErr error
	for _, err := range lmsdk.Query(ctx, promptText(`Call the echo tool with {"text":"blocked"}.`), opts...) {
		if err != nil {
			gotErr = err
			break
		}
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "integration denied") {
		t.Fatalf("expected permission denial, got %v", gotErr)
	}
}
