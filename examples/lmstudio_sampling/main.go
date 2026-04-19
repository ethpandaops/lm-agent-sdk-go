package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	sawText := false
	for msg, err := range sdk.Query(ctx, sdk.Text("Suggest a short commit message and body for adding request retries."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithTemperature(0.2),
		sdk.WithTopP(0.95),
		sdk.WithTopK(40),
		sdk.WithMinP(0.05),
		sdk.WithRepeatPenalty(1.1),
		sdk.WithMaxTokens(250),
		sdk.WithPresencePenalty(0.1),
		sdk.WithFrequencyPenalty(0.1),
		sdk.WithSeed(7),
		sdk.WithStop("###"),
		sdk.WithTTL(30*time.Minute),
		sdk.WithParallelToolCalls(true),
		sdk.WithToolChoice("auto"),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		switch m := msg.(type) {
		case *sdk.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*sdk.TextBlock); ok && strings.TrimSpace(text.Text) != "" {
					sawText = true
					break
				}
			}
		case *sdk.ResultMessage:
			if m.Result != nil && strings.TrimSpace(*m.Result) != "" {
				sawText = true
			}
		}
		exampleutil.DisplayMessage(msg)
	}

	if !sawText {
		fmt.Println("query error: provider returned no textual content for this controls combination")
	}
}
