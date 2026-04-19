// Package main demonstrates running multiple Query() calls concurrently.
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
	"golang.org/x/sync/errgroup"
)

type translationResult struct {
	Style string
	Text  string
	Cost  float64
}

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

	fmt.Println("Parallel Queries Example")
	fmt.Println("Running 3 translation styles concurrently, then picking the best.")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	originalText := "To be, or not to be, that is the question."

	styles := []struct {
		Name   string
		Prompt string
	}{
		{"Formal", "You are a formal translator. Translate to French using literary, formal language."},
		{"Casual", "You are a casual translator. Translate to French using everyday, colloquial language."},
		{"Poetic", "You are a poetic translator. Translate to French preserving rhythm and beauty."},
	}

	var (
		mu      sync.Mutex
		results = make([]translationResult, 0, len(styles))
	)

	g, gCtx := errgroup.WithContext(ctx)

	for _, style := range styles {
		g.Go(func() error {
			text, cost, err := getAssistantText(sdk.Query(gCtx,
				sdk.Text(fmt.Sprintf("Translate this to French: %q", originalText)),
				sdk.WithAPIKey(exampleutil.APIKey()),
				sdk.WithModel(exampleutil.DefaultModel()),
				sdk.WithSystemPrompt(style.Prompt),
				sdk.WithMaxTurns(1),
			))
			if err != nil {
				return fmt.Errorf("%s translation: %w", style.Name, err)
			}

			mu.Lock()
			results = append(results, translationResult{
				Style: style.Name,
				Text:  text,
				Cost:  cost,
			})
			mu.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	var (
		totalCost    float64
		descriptions []string
	)

	for _, r := range results {
		fmt.Printf("[%s] %s\n", r.Style, r.Text)
		totalCost += r.Cost
		descriptions = append(descriptions,
			fmt.Sprintf("- %s: %s", r.Style, r.Text))
	}

	fmt.Printf("\nTranslation cost: $%.8f\n", totalCost)

	// Use a judge query to pick the best
	fmt.Println("\n--- Judge Evaluation ---")

	judgePrompt := fmt.Sprintf(
		"Original: %q\n\nTranslations:\n%s\n\n"+
			"Which translation is best and why? Be concise (2-3 sentences).",
		originalText, strings.Join(descriptions, "\n"),
	)

	judgeText, judgeCost, err := getAssistantText(sdk.Query(ctx,
		sdk.Text(judgePrompt),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSystemPrompt("You are a French language expert. Evaluate translations for accuracy and style."),
		sdk.WithMaxTurns(1),
	))
	if err != nil {
		fmt.Printf("Judge error: %v\n", err)
		return
	}

	fmt.Printf("Judge: %s\n", judgeText)
	fmt.Printf("\nTotal cost (translations + judge): $%.8f\n", totalCost+judgeCost)
}
