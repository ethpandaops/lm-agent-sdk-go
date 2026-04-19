//go:build integration

package integration_test

import (
	"strings"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestStatefulChat_BasicCall(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	resp, err := lmsdk.StatefulChat(ctx, lmsdk.StatefulChatRequest{
		Model:     integrationModel(),
		Input:     "Reply with exactly: OK.",
		Reasoning: lmsdk.ReasoningOff,
	}, integrationOptions()...)
	if err != nil {
		t.Fatalf("stateful chat: %v", err)
	}
	if resp.ResponseID == "" {
		t.Fatal("expected response_id")
	}
	if resp.Stats.InputTokens == 0 {
		t.Fatal("expected non-zero input_tokens")
	}
	if resp.Stats.TokensPerSecond <= 0 {
		t.Errorf("expected positive tokens_per_second, got %v", resp.Stats.TokensPerSecond)
	}
	if text := resp.Text(); text == "" {
		t.Errorf("expected non-empty text reply, got %#v", resp.Output)
	}
}

func TestStatefulChat_Continuation(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	store := true
	first, err := lmsdk.StatefulChat(ctx, lmsdk.StatefulChatRequest{
		Model:     integrationModel(),
		Input:     "Pick a number between 1 and 9. Reply with just that digit.",
		Reasoning: lmsdk.ReasoningOff,
		Store:     &store,
	}, integrationOptions()...)
	if err != nil {
		t.Fatalf("first turn: %v", err)
	}
	if first.ResponseID == "" {
		t.Fatal("need a response_id to continue")
	}

	second, err := lmsdk.StatefulChat(ctx, lmsdk.StatefulChatRequest{
		Model:              integrationModel(),
		Input:              "Double that number. Reply with just the result.",
		PreviousResponseID: first.ResponseID,
		Reasoning:          lmsdk.ReasoningOff,
	}, integrationOptions()...)
	if err != nil {
		t.Fatalf("second turn: %v", err)
	}
	// Continuation should have carried some history — input tokens should
	// exceed the follow-up prompt on its own.
	if second.Stats.InputTokens <= 5 {
		t.Errorf("expected continuation to carry history, got input_tokens=%d", second.Stats.InputTokens)
	}
	if strings.TrimSpace(second.Text()) == "" {
		t.Errorf("expected non-empty reply on turn 2")
	}
}
