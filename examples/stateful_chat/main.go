// Package main demonstrates LM Studio's native stateful chat endpoint
// (/api/v1/chat) — reasoning effort control + previous_response_id
// continuation without re-sending history.
package main

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	store := true

	// Turn 1: establish the topic, ask the server to persist the response.
	first, err := sdk.StatefulChat(ctx, sdk.StatefulChatRequest{
		Model:     exampleutil.DefaultModel(),
		Input:     "Pick a random planet in our solar system. Reply with just its name.",
		Reasoning: sdk.ReasoningOff,
		Store:     &store,
	},
		sdk.WithAPIKey(exampleutil.APIKey()),
	)
	if err != nil {
		fmt.Printf("first turn error: %v\n", err)
		return
	}
	fmt.Printf("Turn 1: %s\n", first.Text())
	fmt.Printf("  response_id=%s  ttft=%.3fs  tok/s=%.1f\n",
		first.ResponseID, first.Stats.TimeToFirstTokenSeconds, first.Stats.TokensPerSecond)

	// Turn 2: continue the conversation without resending any history — the
	// server threads state via previous_response_id.
	second, err := sdk.StatefulChat(ctx, sdk.StatefulChatRequest{
		Model:              exampleutil.DefaultModel(),
		Input:              "How many moons does it have? Just the number.",
		PreviousResponseID: first.ResponseID,
		Reasoning:          sdk.ReasoningOff,
	},
		sdk.WithAPIKey(exampleutil.APIKey()),
	)
	if err != nil {
		fmt.Printf("second turn error: %v\n", err)
		return
	}
	fmt.Printf("Turn 2: %s\n", second.Text())
	fmt.Printf("  input_tokens=%d (includes history)  output_tokens=%d\n",
		second.Stats.InputTokens, second.Stats.TotalOutputTokens)
}
