package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

type Checklist struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	format := map[string]any{
		"type": "json_schema",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{"type": "string"},
				"items": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
			"required": []string{"title", "items"},
		},
	}

	for msg, err := range sdk.Query(ctx, sdk.Text("Create a short Go onboarding checklist."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithOutputFormat(format),
		sdk.WithMaxTurns(2),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
		result, ok := msg.(*sdk.ResultMessage)
		if !ok {
			continue
		}
		if result.StructuredOutput == nil {
			continue
		}
		b, marshalErr := json.Marshal(result.StructuredOutput)
		if marshalErr != nil {
			fmt.Printf("marshal structured output error: %v\n", marshalErr)
			return
		}
		var checklist Checklist
		if unmarshalErr := json.Unmarshal(b, &checklist); unmarshalErr != nil {
			fmt.Printf("unmarshal structured output error: %v\n", unmarshalErr)
			return
		}
		fmt.Printf("Parsed checklist: %+v\n", checklist)
	}
}
