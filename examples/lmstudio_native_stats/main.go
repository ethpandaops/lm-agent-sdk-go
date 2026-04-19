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

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	resp, err := sdk.NativeChatCompletions(ctx,
		"In one short sentence, why do goroutines exist?",
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMaxTokens(80),
		sdk.WithTemperature(0.4),
	)
	if err != nil {
		fmt.Printf("native call error: %v\n", err)
		return
	}

	fmt.Printf("Model: %s (%s / %s)\n", resp.Model, resp.ModelInfo.Arch, resp.ModelInfo.Quant)
	fmt.Printf("Context: %d tokens\n", resp.ModelInfo.ContextLength)
	fmt.Printf("Runtime: %s %s\n", resp.Runtime.Name, resp.Runtime.Version)
	fmt.Printf("Stats: %.1f tok/s, TTFT=%.3fs, gen=%.3fs, stop=%s\n",
		resp.Stats.TokensPerSecond,
		resp.Stats.TimeToFirstToken,
		resp.Stats.GenerationTime,
		resp.Stats.StopReason,
	)
	fmt.Printf("Tokens: %d prompt / %d completion / %d total\n",
		resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)

	if len(resp.Choices) > 0 {
		c := resp.Choices[0]
		if c.Message.Reasoning != "" {
			fmt.Printf("Reasoning: %s\n", c.Message.Reasoning)
		}
		if c.Message.ReasoningContent != "" {
			fmt.Printf("Reasoning (legacy): %s\n", c.Message.ReasoningContent)
		}
		fmt.Printf("Assistant: %s\n", c.Message.Content)
	}
}
