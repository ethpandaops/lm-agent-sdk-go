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

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	messages := sdk.MessagesFromSlice([]sdk.StreamingMessage{
		sdk.NewUserMessage(sdk.Text("Give me 3 practical tips for writing cleaner Go code.")),
		sdk.NewUserMessage(sdk.Text("Now compress those tips into one sentence.")),
	})

	for msg, err := range sdk.QueryStream(ctx, messages,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMaxTurns(3),
	) {
		if err != nil {
			fmt.Printf("query stream error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}
}
