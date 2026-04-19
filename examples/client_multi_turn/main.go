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

	client := sdk.NewClient()
	defer func() { _ = client.Close() }()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMaxTokens(300),
	); err != nil {
		fmt.Printf("client start error: %v\n", err)
		return
	}

	if err := client.Query(ctx, sdk.Text("Give me one Go performance tip in two sentences.")); err != nil {
		fmt.Printf("query 1 error: %v\n", err)
		return
	}
	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("receive 1 error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}

	if err := client.Query(ctx, sdk.Text("Now give one memory optimization tip in two sentences.")); err != nil {
		fmt.Printf("query 2 error: %v\n", err)
		return
	}
	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("receive 2 error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}
}
