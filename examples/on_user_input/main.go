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

	inputRequests := 0

	for msg, err := range sdk.Query(ctx,
		sdk.Text(`Call the stdio tool to ask whether you should continue. If the answer is yes, reply exactly with confirmed.`),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSystemPrompt("Use the stdio tool when instructed. After the tool returns yes, reply exactly with confirmed and do not call any more tools."),
		sdk.WithOnUserInput(func(_ context.Context, req *sdk.UserInputRequest) (*sdk.UserInputResponse, error) {
			inputRequests++
			fmt.Printf("User input requested: %s\n", req.Questions[0].Question)
			return &sdk.UserInputResponse{
				Answers: map[string]*sdk.UserInputAnswer{
					req.Questions[0].ID: {Answers: []string{"yes"}},
				},
			}, nil
		}),
		sdk.WithToolChoice(map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "mcp__sdk__stdio"},
		}),
		sdk.WithMaxTurns(4),
		sdk.WithTemperature(0),
	) {
		if err != nil {
			if inputRequests > 0 && strings.Contains(err.Error(), "max turns reached without terminal response") {
				fmt.Println("The stdio tool round-trip succeeded, but the local model kept re-calling it instead of emitting the terminal response.")
				return
			}
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}
}
