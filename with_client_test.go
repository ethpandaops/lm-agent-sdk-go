package lmsdk

import (
	"context"
	"testing"
	"time"
)

func TestWithClient(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WithClient(ctx, func(c Client) error {
		return c.Query(ctx, Text("hello"))
	}, WithTransport(&scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
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
	}), WithMaxTurns(1))
	if err != nil {
		t.Fatalf("with client: %v", err)
	}
}
