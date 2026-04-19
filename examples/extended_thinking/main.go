// Package main demonstrates extended thinking capabilities with LM Studio.
//
// Extended thinking allows models to "think through" complex problems before
// responding, providing transparency into the reasoning process.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

// displayMessageBasic handles message display for non-streaming mode.
func displayMessageBasic(msg sdk.Message) {
	switch m := msg.(type) {
	case *sdk.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case *sdk.ThinkingBlock:
				fmt.Println("[Thinking]")
				fmt.Println(b.Thinking)
				fmt.Println("[End Thinking]")
				fmt.Println()
			case *sdk.TextBlock:
				fmt.Printf("Assistant: %s\n", b.Text)
			}
		}
	case *sdk.ResultMessage:
		fmt.Println()
		fmt.Println("=== Result ===")
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.6f\n", *m.TotalCostUSD)
		}
	}
}

// displayMessageStreaming handles message display for streaming mode.
func displayMessageStreaming(msg sdk.Message) {
	switch m := msg.(type) {
	case *sdk.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case *sdk.ThinkingBlock:
				fmt.Print(b.Thinking)
			case *sdk.TextBlock:
				fmt.Print(b.Text)
			}
		}
	case *sdk.StreamEvent:
		eventType, ok := m.Event["type"].(string)
		if !ok {
			return
		}

		switch eventType {
		case "content_block_start":
			if cb, ok := m.Event["content_block"].(map[string]any); ok {
				if cbType, ok := cb["type"].(string); ok {
					switch cbType {
					case "thinking":
						fmt.Print("[Thinking] ")
					case "text":
						fmt.Print("[Response] ")
					}
				}
			}
		case "content_block_delta":
			if delta, ok := m.Event["delta"].(map[string]any); ok {
				if thinking, ok := delta["thinking"].(string); ok {
					fmt.Print(thinking)
				}
				if text, ok := delta["text"].(string); ok {
					fmt.Print(text)
				}
			}
		case "content_block_stop":
			fmt.Println()
		case "message_stop":
			fmt.Println()
		}
	case *sdk.ResultMessage:
		fmt.Println("=== Result ===")
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.6f\n", *m.TotalCostUSD)
		}
	}
}

func exampleBasicThinking() {
	fmt.Println("=== Basic Extended Thinking Example ===")
	fmt.Println("Thinking is shown after completion (using WithThinking).")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	client := sdk.NewClient()
	defer func() { _ = client.Close() }()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithThinking(sdk.ThinkingConfigEnabled{BudgetTokens: 8000}),
		sdk.WithMaxTurns(1),
	); err != nil {
		fmt.Printf("start error: %v\n", err)
		return
	}

	prompt := "What is the sum of the first 20 prime numbers? Show your reasoning."
	fmt.Printf("Prompt: %s\n", prompt)
	fmt.Println(strings.Repeat("-", 50))

	if err := client.Query(ctx, sdk.Text(prompt)); err != nil {
		fmt.Printf("query error: %v\n", err)
		return
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("receive error: %v\n", err)
			return
		}
		displayMessageBasic(msg)
	}

	fmt.Println()
}

func exampleThinkingConfig() {
	fmt.Println("=== ThinkingConfig with Effort Example ===")
	fmt.Println("Uses adaptive thinking plus WithEffort for control.")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	client := sdk.NewClient()
	defer func() { _ = client.Close() }()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithThinking(sdk.ThinkingConfigAdaptive{}),
		sdk.WithEffort(sdk.EffortHigh),
		sdk.WithMaxTurns(1),
	); err != nil {
		fmt.Printf("start error: %v\n", err)
		return
	}

	prompt := "Explain the relationship between the Fibonacci sequence and the golden ratio."
	fmt.Printf("Prompt: %s\n", prompt)
	fmt.Println(strings.Repeat("-", 50))

	if err := client.Query(ctx, sdk.Text(prompt)); err != nil {
		fmt.Printf("query error: %v\n", err)
		return
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("receive error: %v\n", err)
			return
		}
		displayMessageBasic(msg)
	}

	fmt.Println()
}

func exampleStreamingThinking() {
	fmt.Println("=== Streaming Extended Thinking Example ===")
	fmt.Println("Thinking blocks are displayed in real-time as they stream.")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	client := sdk.NewClient()
	defer func() { _ = client.Close() }()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithThinking(sdk.ThinkingConfigEnabled{BudgetTokens: 8000}),
		sdk.WithIncludePartialMessages(true),
		sdk.WithMaxTurns(1),
	); err != nil {
		fmt.Printf("start error: %v\n", err)
		return
	}

	prompt := "If a train leaves Chicago at 9am traveling 60mph, and another train " +
		"leaves New York at 10am traveling 80mph toward Chicago, and they are " +
		"790 miles apart, at what time will they meet?"
	fmt.Printf("Prompt: %s\n", prompt)
	fmt.Println(strings.Repeat("-", 50))

	if err := client.Query(ctx, sdk.Text(prompt)); err != nil {
		fmt.Printf("query error: %v\n", err)
		return
	}

	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			break
		}
		displayMessageStreaming(msg)
		if _, ok := msg.(*sdk.ResultMessage); ok {
			break
		}
	}

	fmt.Println()
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	fmt.Println("Extended Thinking Examples")
	fmt.Println("Note: Requires a model that supports extended thinking.")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	examples := map[string]func(){
		"basic":     exampleBasicThinking,
		"thinking":  exampleThinkingConfig,
		"streaming": exampleStreamingThinking,
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <example_name>")
		fmt.Println("\nAvailable examples:")
		fmt.Println("  all       - Run all examples")
		fmt.Println("  basic     - Show thinking after completion (WithThinking)")
		fmt.Println("  thinking  - Show thinking with ThinkingConfig and Effort")
		fmt.Println("  streaming - Stream thinking in real-time")
		return
	}

	exampleName := os.Args[1]

	if exampleName == "all" {
		exampleBasicThinking()
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println()
		exampleThinkingConfig()
		fmt.Println(strings.Repeat("-", 60))
		fmt.Println()
		exampleStreamingThinking()
	} else if fn, ok := examples[exampleName]; ok {
		fn()
	} else {
		fmt.Printf("Error: Unknown example '%s'\n", exampleName)
		fmt.Println("\nAvailable examples: all, basic, thinking, streaming")
		os.Exit(1)
	}
}
