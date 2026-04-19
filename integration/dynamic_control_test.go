//go:build integration

package integration_test

import (
	"errors"
	"strings"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestUnsupportedControlsAndInterrupt(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client := lmsdk.NewClient()
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithIncludePartialMessages(true))
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	for _, err := range []error{
		client.ReconnectMCPServer(ctx, "srv"),
		client.ToggleMCPServer(ctx, "srv", true),
		client.StopTask(ctx, "task-1"),
		client.SendToolResult(ctx, "tool-1", "{}", false),
	} {
		var unsupported *lmsdk.UnsupportedControlError
		if !errors.As(err, &unsupported) {
			t.Fatalf("expected UnsupportedControlError, got %T", err)
		}
	}

	if err := client.Query(ctx, promptText("Write a long answer about distributed systems in at least 30 bullet points.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	var sawAssistant bool
	var sawResult bool
	var gotErr error

	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			gotErr = err
			break
		}
		switch msg.(type) {
		case *lmsdk.StreamEvent:
			if !sawAssistant {
				sawAssistant = true
				if err := client.Interrupt(ctx); err != nil {
					t.Fatalf("interrupt: %v", err)
				}
			}
		case *lmsdk.AssistantMessage:
			if !sawAssistant {
				sawAssistant = true
				if err := client.Interrupt(ctx); err != nil {
					t.Fatalf("interrupt: %v", err)
				}
			}
		case *lmsdk.ResultMessage:
			sawResult = true
		}
	}

	if !sawAssistant {
		t.Fatal("expected at least one assistant message before interrupt")
	}
	if sawResult {
		t.Fatal("expected interrupt to stop the response before a final result message")
	}
	if gotErr == nil {
		t.Fatal("expected interrupt to surface an error")
	}
	if !strings.Contains(strings.ToLower(gotErr.Error()), "canceled") && !strings.Contains(strings.ToLower(gotErr.Error()), "cancelled") {
		t.Fatalf("expected cancellation-related error, got %v", gotErr)
	}
}
