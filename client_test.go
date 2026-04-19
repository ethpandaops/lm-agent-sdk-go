package lmsdk

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClientLifecycleAndUnsupportedControls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := NewClient()
	if err := client.Start(ctx, WithTransport(&scriptedTransport{t: t}), WithAPIKey("unused")); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SetModel(ctx, nil); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := client.SetPermissionMode(ctx, "acceptEdits"); err != nil {
		t.Fatalf("set permission mode: %v", err)
	}

	checks := []error{
		client.ReconnectMCPServer(ctx, "srv"),
		client.ToggleMCPServer(ctx, "srv", true),
		client.StopTask(ctx, "task-1"),
		client.SendToolResult(ctx, "tool-1", "{}", false),
	}
	for _, err := range checks {
		var unsupported *UnsupportedControlError
		if !errors.As(err, &unsupported) {
			t.Fatalf("expected unsupported control error, got %T", err)
		}
	}
}
