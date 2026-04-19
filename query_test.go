package lmsdk

import (
	"context"
	"testing"
	"time"
)

func TestQueryStream_YieldsAcrossMultipleResults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "first"},
								"finish_reason": "stop",
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
								"delta":         map[string]any{"content": "second"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	stream := MessagesFromSlice([]StreamingMessage{
		NewUserMessage(Text("hello")),
		NewUserMessage(Text("again")),
	})

	var results int
	for msg, err := range QueryStream(ctx, stream, WithTransport(tr), WithMaxTurns(1)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := msg.(*ResultMessage); ok {
			results++
		}
	}

	if results != 2 {
		t.Fatalf("expected 2 result messages from multi-turn stream, got %d", results)
	}
}
