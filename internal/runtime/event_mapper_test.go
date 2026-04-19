package runtime

import (
	"strings"
	"testing"
)

func TestParseChunkChat(t *testing.T) {
	raw := map[string]any{
		"choices": []any{
			map[string]any{
				"delta": map[string]any{
					"content": "hello",
					"tool_calls": []any{
						map[string]any{
							"index": 1.0,
							"id":    "call_1",
							"function": map[string]any{
								"name":      "mcp__calc__add",
								"arguments": "{\"a\":1}",
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
	}

	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Content != "hello" {
		t.Fatalf("unexpected content: %q", chunks[0].Content)
	}
	if chunks[0].Finish != "tool_calls" {
		t.Fatalf("unexpected finish: %q", chunks[0].Finish)
	}
	if len(chunks[0].ToolDeltas) != 1 {
		t.Fatalf("expected 1 tool delta, got %d", len(chunks[0].ToolDeltas))
	}
	if chunks[0].ToolDeltas[0].Name != "mcp__calc__add" {
		t.Fatalf("unexpected tool name: %q", chunks[0].ToolDeltas[0].Name)
	}
}

func TestParseChunkResponsesTextDelta(t *testing.T) {
	raw := map[string]any{"type": "response.output_text.delta", "delta": "hello"}
	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Content != "hello" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestParseChunkResponsesErrors(t *testing.T) {
	cases := []map[string]any{
		{"type": "response.error", "error": map[string]any{"message": "bad request"}},
		{"type": "response.failed", "error": map[string]any{"message": "failed"}},
		{"type": "error", "error": map[string]any{"message": "boom"}},
	}
	for _, raw := range cases {
		if _, err := parseChunk(raw); err == nil {
			t.Fatalf("expected error for %#v", raw)
		}
	}
}

func TestParseChunkChatStreamError(t *testing.T) {
	// Upstream provider 429 sent inside the SSE stream (no choices, no type).
	raw := map[string]any{
		"error": map[string]any{
			"message": "rate-limited upstream",
			"code":    float64(429),
			"metadata": map[string]any{
				"provider_name": "Venice",
			},
		},
	}

	_, err := parseChunk(raw)
	if err == nil {
		t.Fatal("expected error for in-stream 429")
	}
	if !strings.Contains(err.Error(), "rate-limited upstream") {
		t.Fatalf("expected error message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected error code in message, got: %v", err)
	}
}

func TestParseChunkResponsesOutputItemFunctionCall(t *testing.T) {
	raw := map[string]any{
		"type":         "response.output_item.added",
		"output_index": 2.0,
		"item": map[string]any{
			"type":      "function_call",
			"call_id":   "call_abc",
			"name":      "mcp__srv__echo",
			"arguments": map[string]any{"x": 1.0},
		},
	}

	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || len(chunks[0].ToolDeltas) != 1 {
		t.Fatalf("expected one tool delta, got %#v", chunks)
	}
	td := chunks[0].ToolDeltas[0]
	if td.Index != 2 || td.ID != "call_abc" || td.Name != "mcp__srv__echo" {
		t.Fatalf("unexpected tool delta: %#v", td)
	}
	if td.Args == "" {
		t.Fatalf("expected serialized args, got empty: %#v", td)
	}
}

func TestParseChunkResponsesFunctionCallDone(t *testing.T) {
	raw := map[string]any{
		"type":         "response.function_call_arguments.done",
		"output_index": 0.0,
		"call_id":      "call_done",
		"name":         "mcp__srv__echo",
		"arguments":    map[string]any{"ok": true},
	}

	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || len(chunks[0].ToolDeltas) != 1 {
		t.Fatalf("expected one tool delta, got %#v", chunks)
	}
	td := chunks[0].ToolDeltas[0]
	if td.Args == "" {
		t.Fatalf("expected args to be serialized: %#v", td)
	}
}

func TestParseChunkResponsesOutputTextDoneWithText(t *testing.T) {
	raw := map[string]any{"type": "response.output_text.done", "text": "final"}
	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Content != "final" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestParseChunkResponsesRefusalEvents(t *testing.T) {
	delta := map[string]any{"type": "response.refusal.delta", "delta": "no"}
	chunks, err := parseChunk(delta)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Content != "no" {
		t.Fatalf("unexpected refusal delta: %#v", chunks)
	}

	done := map[string]any{"type": "response.refusal.done", "refusal": "nope"}
	chunks, err = parseChunk(done)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Content != "nope" {
		t.Fatalf("unexpected refusal done: %#v", chunks)
	}
}

func TestParseChunkResponsesOutputItemMessageText(t *testing.T) {
	raw := map[string]any{
		"type": "response.output_item.done",
		"item": map[string]any{
			"type": "message",
			"content": []any{
				map[string]any{"type": "output_text", "text": "a"},
				map[string]any{"type": "output_text", "text": "b"},
			},
		},
	}
	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Content != "ab" {
		t.Fatalf("unexpected message chunks: %#v", chunks)
	}
}

func TestParseChunkChatImages(t *testing.T) {
	raw := map[string]any{
		"choices": []any{
			map[string]any{
				"delta": map[string]any{
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
	}

	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || len(chunks[0].Images) != 1 {
		t.Fatalf("expected one image chunk, got %#v", chunks)
	}
	if chunks[0].Images[0].URL != "data:image/png;base64,aGVsbG8=" {
		t.Fatalf("unexpected image URL: %#v", chunks[0].Images[0])
	}
}

func TestParseChunkChatCompletionJSONMessage(t *testing.T) {
	raw := map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"role": "assistant",
					"content": []any{
						map[string]any{
							"type": "text",
							"text": "done",
						},
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
	}

	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected one chunk, got %#v", chunks)
	}
	if chunks[0].Content != "done" {
		t.Fatalf("unexpected content: %#v", chunks[0])
	}
	if chunks[0].Finish != "stop" {
		t.Fatalf("unexpected finish: %#v", chunks[0])
	}
	if len(chunks[0].Images) != 1 {
		t.Fatalf("expected one image, got %#v", chunks[0])
	}
	if chunks[0].Images[0].MediaType != "image/png" {
		t.Fatalf("unexpected media type: %#v", chunks[0].Images[0])
	}
}

func TestParseChunkResponsesOutputItemMessageImage(t *testing.T) {
	raw := map[string]any{
		"type": "response.output_item.done",
		"item": map[string]any{
			"type": "message",
			"content": []any{
				map[string]any{
					"type": "output_image",
					"image_url": map[string]any{
						"url": "data:image/png;base64,aGVsbG8=",
					},
					"media_type": "image/png",
				},
			},
		},
	}

	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || len(chunks[0].Images) != 1 {
		t.Fatalf("expected one image chunk, got %#v", chunks)
	}
	if chunks[0].Images[0].MediaType != "image/png" {
		t.Fatalf("unexpected image media type: %#v", chunks[0].Images[0])
	}
}

func TestParseChunkResponsesImageGenerationCompleted(t *testing.T) {
	raw := map[string]any{
		"type": "response.image_generation_call.completed",
		"image_url": map[string]any{
			"url": "data:image/png;base64,aGVsbG8=",
		},
	}

	chunks, err := parseChunk(raw)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || len(chunks[0].Images) != 1 {
		t.Fatalf("expected image chunk, got %#v", chunks)
	}
}

func TestParseChunkResponsesLifecycleAndIgnoredEvents(t *testing.T) {
	ignored := []map[string]any{
		{"type": "response.created"},
		{"type": "response.in_progress"},
		{"type": "response.reasoning_text.delta", "delta": "thinking"},
		{"type": "response.reasoning_summary_part.done"},
		{"type": "response.output_text.annotation.added"},
		{"type": "response.image_generation_call.completed"},
	}
	for _, raw := range ignored {
		chunks, err := parseChunk(raw)
		if err != nil {
			t.Fatalf("unexpected error for %#v: %v", raw, err)
		}
		if len(chunks) != 0 {
			t.Fatalf("expected ignored event to produce no chunks, got %#v", chunks)
		}
	}

	incomplete := map[string]any{"type": "response.incomplete"}
	chunks, err := parseChunk(incomplete)
	if err != nil {
		t.Fatalf("parseChunk error: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Finish != "length" {
		t.Fatalf("expected length finish, got %#v", chunks)
	}
}
