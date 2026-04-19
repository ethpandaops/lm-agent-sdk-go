//go:build integration

package integration_test

import (
	"strings"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestPartialMessages_StreamEventsAlwaysEmitted(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	// StreamEvents are always emitted regardless of IncludePartialMessages.
	var (
		sawStreamEvent bool
		sawResult      bool
	)

	for msg, err := range lmsdk.Query(ctx, promptText("Say hi."), integrationOptions()...) {
		if err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "Client.Timeout") {
				t.Skipf("skipping due to timeout: %v", err)
			}
			t.Fatalf("query error: %v", err)
		}

		switch msg.(type) {
		case *lmsdk.StreamEvent:
			sawStreamEvent = true
		case *lmsdk.ResultMessage:
			sawResult = true
		}
	}

	if !sawStreamEvent {
		t.Fatal("expected StreamEvent messages")
	}
	if !sawResult {
		t.Fatal("expected ResultMessage")
	}
}

func TestPartialMessages_PartialAssistantMessages(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithIncludePartialMessages(true))

	var sawPartialAssistant bool

	for msg, err := range lmsdk.Query(ctx, promptText("Say hi."), opts...) {
		if err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "Client.Timeout") {
				t.Skipf("skipping due to timeout: %v", err)
			}
			t.Fatalf("query error: %v", err)
		}

		// With IncludePartialMessages, we should see partial AssistantMessages
		// emitted as content arrives (before the final ResultMessage).
		if _, ok := msg.(*lmsdk.AssistantMessage); ok {
			sawPartialAssistant = true
		}
	}

	if !sawPartialAssistant {
		t.Fatal("expected partial AssistantMessages with IncludePartialMessages enabled")
	}
}

func TestPartialMessages_NoPartialAssistantWithoutFlag(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	// Without IncludePartialMessages, we should only see the final
	// AssistantMessage (not partial streaming ones). StreamEvents still appear.
	var assistantCount int
	for msg, err := range lmsdk.Query(ctx, promptText("Say hi."), integrationOptions()...) {
		if err != nil {
			if strings.Contains(err.Error(), "context deadline exceeded") || strings.Contains(err.Error(), "Client.Timeout") {
				t.Skipf("skipping due to timeout: %v", err)
			}
			t.Fatalf("query error: %v", err)
		}
		if _, ok := msg.(*lmsdk.AssistantMessage); ok {
			assistantCount++
		}
	}

	// Without the flag we expect at most 1 final AssistantMessage (the aggregated one).
	// With the flag we'd see many partial ones.
	if assistantCount > 1 {
		t.Fatalf("expected at most 1 AssistantMessage without IncludePartialMessages, got %d", assistantCount)
	}
}
