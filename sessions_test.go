package lmsdk

import (
	"context"
	"testing"
	"time"
)

func TestGetSessionMessages_PreservesAssistantToolCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := t.TempDir() + "/sessions.json"
	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta": map[string]any{
									"tool_calls": []any{
										map[string]any{
											"index": 0.0,
											"id":    "call_1",
											"function": map[string]any{
												"name":      "mcp__sdk__echo",
												"arguments": `{"text":"hello"}`,
											},
										},
									},
								},
								"finish_reason": "tool_calls",
							},
						},
					},
				}, nil
			},
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "done"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	tool := NewTool("echo", "Echo text", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"echo": input["text"]}, nil
	})

	for _, err := range Query(ctx, Text("use the tool"), WithTransport(tr), WithSDKTools(tool), WithSessionStorePath(store), WithMaxTurns(3)) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	msgs, err := GetSessionMessages(ctx, "default", WithSessionStorePath(store))
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	var sawToolUse bool
	var sawToolResult bool
	for _, msg := range msgs {
		assistant, ok := msg.(*AssistantMessage)
		if !ok {
			continue
		}
		for _, block := range assistant.Content {
			switch b := block.(type) {
			case *ToolUseBlock:
				sawToolUse = sawToolUse || (b.Name == "mcp__sdk__echo" && b.Input["text"] == "hello")
			case *ToolResultBlock:
				sawToolResult = sawToolResult || (b.ToolUseID == "call_1")
			}
		}
	}
	if !sawToolUse || !sawToolResult {
		t.Fatalf("expected persisted tool use/result messages, got %#v", msgs)
	}
}

func TestGetSessionMessages_PreservesAssistantImages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := t.TempDir() + "/sessions.json"
	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta": map[string]any{
									"content": "icon concept",
									"images": []any{
										map[string]any{
											"type": "image_url",
											"image_url": map[string]any{
												"url": "data:image/png;base64,aGVsbG8=",
											},
											"media_type": "image/png",
										},
									},
								},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("draw an icon"), WithTransport(tr), WithSessionStorePath(store), WithMaxTurns(1)) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	msgs, err := GetSessionMessages(ctx, "default", WithSessionStorePath(store))
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	var sawImage bool
	for _, msg := range msgs {
		assistant, ok := msg.(*AssistantMessage)
		if !ok {
			continue
		}
		for _, block := range assistant.Content {
			image, ok := block.(*ImageBlock)
			if !ok {
				continue
			}
			if image.MediaType == "image/png" && image.URL == "data:image/png;base64,aGVsbG8=" {
				sawImage = true
			}
		}
	}
	if !sawImage {
		t.Fatalf("expected persisted image message, got %#v", msgs)
	}
}
