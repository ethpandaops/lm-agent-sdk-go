// Package main demonstrates multi-step LLM orchestration with Go control flow.
//
// Shows a Generate -> Evaluate -> Refine pipeline where Go code
// gates transitions between LLM calls.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func getAssistantText(
	msgs func(func(sdk.Message, error) bool),
) (string, float64, error) {
	var (
		text string
		cost float64
	)

	for msg, err := range msgs {
		if err != nil {
			return "", 0, fmt.Errorf("query: %w", err)
		}

		if m, ok := msg.(*sdk.AssistantMessage); ok {
			for _, block := range m.Content {
				if tb, ok := block.(*sdk.TextBlock); ok {
					text = tb.Text
				}
			}
		}

		if m, ok := msg.(*sdk.ResultMessage); ok {
			if m.TotalCostUSD != nil {
				cost = *m.TotalCostUSD
			}
		}
	}

	return text, cost, nil
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	fmt.Println("Pipeline Example")
	fmt.Println("Generate -> Evaluate -> Refine with Go-side gating between LLM calls.")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	opts := []sdk.Option{
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMaxTurns(1),
	}

	var totalCost float64

	// Step 1: Generate
	fmt.Println("--- Step 1: Generate ---")

	draft, cost, err := getAssistantText(sdk.Query(ctx,
		sdk.Text("Write a short product description (2-3 sentences) for a smart water bottle "+
			"that tracks hydration and syncs with a phone app."),
		append(opts, sdk.WithSystemPrompt("You are a marketing copywriter. Write concise, compelling copy."))...,
	))
	if err != nil {
		fmt.Printf("Generate error: %v\n", err)
		return
	}

	totalCost += cost
	fmt.Printf("Draft: %s\n\n", draft)

	// Step 2: Evaluate (Go-side gating + LLM evaluation)
	fmt.Println("--- Step 2: Evaluate ---")

	if len(draft) < 20 {
		fmt.Println("GATE FAILED: Draft too short, skipping evaluation.")
		return
	}

	if !strings.ContainsAny(draft, ".!?") {
		fmt.Println("GATE FAILED: Draft has no sentence endings, skipping evaluation.")
		return
	}

	fmt.Println("Gate passed: draft meets minimum quality criteria.")

	evaluatePrompt := fmt.Sprintf(
		"Evaluate this product description on a scale of 1-10:\n\n%q\n\n"+
			"Respond with exactly this format:\n"+
			"Score: N\nStrengths: ...\nWeaknesses: ...",
		draft,
	)

	evaluation, cost, err := getAssistantText(sdk.Query(ctx,
		sdk.Text(evaluatePrompt),
		append(opts, sdk.WithSystemPrompt("You are a marketing expert who evaluates copy. Be specific and constructive."))...,
	))
	if err != nil {
		fmt.Printf("Evaluate error: %v\n", err)
		return
	}

	totalCost += cost
	fmt.Printf("Evaluation:\n%s\n\n", evaluation)

	// Go-side decision: skip refinement if score is high enough
	if strings.Contains(evaluation, "Score: 9") || strings.Contains(evaluation, "Score: 10") {
		fmt.Println("Score is 9+, no refinement needed!")
		fmt.Printf("\nTotal cost: $%.8f\n", totalCost)
		return
	}

	// Step 3: Refine
	fmt.Println("--- Step 3: Refine ---")

	refinePrompt := fmt.Sprintf(
		"Here is a product description:\n\n%q\n\n"+
			"Here is feedback on it:\n\n%s\n\n"+
			"Rewrite the description addressing the weaknesses while keeping the strengths. "+
			"Keep it to 2-3 sentences.",
		draft, evaluation,
	)

	refined, cost, err := getAssistantText(sdk.Query(ctx,
		sdk.Text(refinePrompt),
		append(opts, sdk.WithSystemPrompt("You are a marketing copywriter. Improve copy based on feedback."))...,
	))
	if err != nil {
		fmt.Printf("Refine error: %v\n", err)
		return
	}

	totalCost += cost
	fmt.Printf("Refined: %s\n", refined)
	fmt.Printf("\nTotal cost: $%.8f\n", totalCost)
}
