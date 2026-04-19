package main

import (
	"context"
	"fmt"
	"math"
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

	sqrtTool := sdk.NewTool(
		"sqrt",
		"Compute the square root of a non-negative number.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x": map[string]any{"type": "number", "description": "non-negative input"},
			},
			"required": []string{"x"},
		},
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			x, _ := input["x"].(float64)
			if x < 0 {
				return nil, fmt.Errorf("cannot sqrt a negative number")
			}
			return map[string]any{"result": math.Sqrt(x)}, nil
		},
	)

	addTool := sdk.NewTool(
		"add",
		"Add two numbers together.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "number"},
				"b": map[string]any{"type": "number"},
			},
			"required": []string{"a", "b"},
		},
		func(_ context.Context, input map[string]any) (map[string]any, error) {
			a, _ := input["a"].(float64)
			b, _ := input["b"].(float64)
			return map[string]any{"result": a + b}, nil
		},
	)

	agent := &sdk.Agent{
		Model:     exampleutil.DefaultModel(),
		Tools:     []sdk.Tool{sqrtTool, addTool},
		MaxRounds: 6,
		ExtraOptions: []sdk.Option{
			sdk.WithAPIKey(exampleutil.APIKey()),
			sdk.WithSystemPrompt("You are a calculator agent. Prefer the provided tools when a calculation is requested, then summarise the final answer in one sentence."),
			sdk.WithTemperature(0.1),
			sdk.WithMaxTokens(200),
		},
	}

	result, err := agent.Act(ctx, "Compute sqrt(144) and then add that to 7. Tell me the final number.")
	if err != nil {
		fmt.Printf("act error: %v\n", err)
		return
	}

	fmt.Printf("Rounds: %d, tool calls: %d, stop=%s\n", result.Rounds, result.ToolCalls, result.StopReason)
	fmt.Printf("Final answer: %s\n", result.Text)
}
