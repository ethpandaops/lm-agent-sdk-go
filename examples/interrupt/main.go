package main

import (
	"context"
	"errors"
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
	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithIncludePartialMessages(true),
	); err != nil {
		fmt.Printf("client start error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, sdk.Text("Write a very long tutorial on Go interfaces with many sections and examples.")); err != nil {
		fmt.Printf("query error: %v\n", err)
		return
	}

	interrupted := false
	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			if interrupted && errors.Is(err, context.Canceled) {
				fmt.Println("Interrupt completed as expected.")
				fmt.Println("Stream ended after interrupt as expected.")
				return
			}
			fmt.Printf("stream ended after interrupt: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
		if !interrupted {
			if err := client.Interrupt(ctx); err != nil {
				fmt.Printf("interrupt error: %v\n", err)
				return
			}
			fmt.Println("Interrupt requested.")
			interrupted = true
			time.Sleep(100 * time.Millisecond)
		}
	}
}
