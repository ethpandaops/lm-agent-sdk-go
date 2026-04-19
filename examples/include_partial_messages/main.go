// Package main demonstrates partial message streaming where incremental
// assistant updates are received as the model generates responses.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func displayMessage(msg sdk.Message) {
	switch m := msg.(type) {
	case *sdk.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case *sdk.TextBlock:
				fmt.Print(b.Text)
			case *sdk.ThinkingBlock:
				fmt.Print(b.Thinking)
			}
		}
	case *sdk.ResultMessage:
		fmt.Println()
		fmt.Println("Result ended")
		if m.Result != nil {
			fmt.Printf("Final text length: %d chars\n", len(*m.Result))
		}
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.8f\n", *m.TotalCostUSD)
		}
	}
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	fmt.Println("Partial Message Streaming Example")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println("\nThis feature allows you to receive incremental assistant")
	fmt.Println("updates as the model generates responses.")
	fmt.Println(strings.Repeat("=", 50))

	client := sdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	defer func() { _ = client.Close() }()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithIncludePartialMessages(true),
		sdk.WithMaxTurns(2),
	); err != nil {
		fmt.Printf("start error: %v\n", err)
		return
	}

	prompt := "Tell me a short joke, then explain why it's funny."
	fmt.Printf("\nPrompt: %s\n", prompt)
	fmt.Println(strings.Repeat("=", 50))

	if err := client.Query(ctx, sdk.Text(prompt)); err != nil {
		fmt.Printf("query error: %v\n", err)
		return
	}

	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			break
		}
		displayMessage(msg)
		if _, ok := msg.(*sdk.ResultMessage); ok {
			break
		}
	}
}
