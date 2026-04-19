// Package main demonstrates configuring system prompts.
package main

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func noSystemPrompt() {
	fmt.Println("=== No System Prompt (Default) ===")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for msg, err := range sdk.Query(ctx, sdk.Text("What is 2 + 2? Answer briefly."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMaxTurns(1),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}

	fmt.Println()
}

func withStringSystemPrompt() {
	fmt.Println("=== String System Prompt (Pirate) ===")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for msg, err := range sdk.Query(ctx, sdk.Text("What is 2 + 2? Answer briefly."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSystemPrompt("You are a pirate assistant. Respond in pirate speak."),
		sdk.WithMaxTurns(1),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}

	fmt.Println()
}

func withPresetSystemPrompt() {
	fmt.Println("=== Preset System Prompt ===")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for msg, err := range sdk.Query(ctx, sdk.Text("What is 2 + 2? Answer briefly."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSystemPromptPreset(&sdk.SystemPromptPreset{
			Type:   "preset",
			Preset: "default",
		}),
		sdk.WithMaxTurns(1),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}

	fmt.Println()
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	fmt.Println("System Prompt Examples")
	fmt.Println()

	noSystemPrompt()
	withStringSystemPrompt()
	withPresetSystemPrompt()
}
