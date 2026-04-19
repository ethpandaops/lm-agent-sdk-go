// Package main demonstrates API cost control with budget limits.
package main

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func displayMessage(msg sdk.Message) {
	switch m := msg.(type) {
	case *sdk.AssistantMessage:
		for _, block := range m.Content {
			if textBlock, ok := block.(*sdk.TextBlock); ok {
				fmt.Printf("Assistant: %s\n", textBlock.Text)
			}
		}
	case *sdk.ResultMessage:
		if m.TotalCostUSD != nil {
			fmt.Printf("Total cost: $%.8f\n", *m.TotalCostUSD)
		}
		fmt.Printf("Status: %s\n", m.Subtype)
		if m.Subtype == "error_max_budget_usd" {
			fmt.Println("Budget limit exceeded!")
			fmt.Println("Note: The cost may exceed the budget by up to one API call's worth")
		}
	}
}

func withoutBudget() {
	fmt.Println("=== Without Budget Limit ===")

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
		displayMessage(msg)
	}

	fmt.Println()
}

func withReasonableBudget() {
	fmt.Println("=== With Reasonable Budget ($0.10) ===")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for msg, err := range sdk.Query(ctx, sdk.Text("What is 2 + 2? Answer briefly."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMaxBudgetUSD(0.10),
		sdk.WithMaxTurns(1),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		displayMessage(msg)
	}

	fmt.Println()
}

func withTightBudget() {
	fmt.Println("=== With Tight Budget ($0.0001) ===")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for msg, err := range sdk.Query(ctx,
		sdk.Text("Write a detailed essay about the history of computing, covering all major milestones."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMaxBudgetUSD(0.0001),
		sdk.WithMaxTurns(4),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		displayMessage(msg)
	}

	fmt.Println()
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	fmt.Println("Max Budget USD Examples")
	fmt.Println()
	fmt.Println("This example demonstrates using max_budget_usd to control API costs.")
	fmt.Println()

	withoutBudget()
	withReasonableBudget()
	withTightBudget()

	fmt.Println("Note: Budget checking happens after each API call completes,")
	fmt.Println("so the final cost may slightly exceed the specified budget.")
}
