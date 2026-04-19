package hook

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDispatcherRunAppliesMatcherTimeout(t *testing.T) {
	timeout := 0.02 // seconds
	d := NewDispatcher(map[Event][]*Matcher{
		EventUserPromptSubmit: {
			{
				Timeout: &timeout,
				Hooks: []Callback{
					func(ctx context.Context, _ Input, _ *string, _ *Context) (JSONOutput, error) {
						<-ctx.Done()
						return nil, ctx.Err()
					},
				},
			},
		},
	})

	start := time.Now()
	_, err := d.Run(
		context.Background(),
		EventUserPromptSubmit,
		"",
		&UserPromptSubmitInput{HookEventName: string(EventUserPromptSubmit)},
		nil,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("timeout did not trigger promptly, elapsed=%s", elapsed)
	}
}

func TestDispatcherRunWithoutTimeoutSucceeds(t *testing.T) {
	d := NewDispatcher(map[Event][]*Matcher{
		EventUserPromptSubmit: {
			{
				Hooks: []Callback{
					func(context.Context, Input, *string, *Context) (JSONOutput, error) {
						return &SyncJSONOutput{}, nil
					},
				},
			},
		},
	})

	out, err := d.Run(
		context.Background(),
		EventUserPromptSubmit,
		"",
		&UserPromptSubmitInput{HookEventName: string(EventUserPromptSubmit)},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("expected one output, got %d", len(out))
	}
}
