package main

import (
	"context"
	"fmt"
	"os"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	store, err := os.CreateTemp("", "lm-agent-sdk-sessions-*.json")
	if err != nil {
		fmt.Printf("create temp store: %v\n", err)
		return
	}
	_ = store.Close()
	defer func() { _ = os.Remove(store.Name()) }()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	opts := []sdk.Option{
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSessionStorePath(store.Name()),
		sdk.WithMaxTurns(2),
	}
	for msg, err := range sdk.Query(ctx, sdk.Text("Reply with the exact word stored."), opts...) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}

	stat, err := sdk.StatSession(ctx, "default", opts...)
	if err != nil {
		fmt.Printf("stat session: %v\n", err)
		return
	}
	fmt.Printf("Session stat: %+v\n", stat)

	sessions, err := sdk.ListSessions(ctx, opts...)
	if err != nil {
		fmt.Printf("list sessions: %v\n", err)
		return
	}
	fmt.Printf("Sessions in store: %d\n", len(sessions))

	msgs, err := sdk.GetSessionMessages(ctx, "default", opts...)
	if err != nil {
		fmt.Printf("get session messages: %v\n", err)
		return
	}
	fmt.Printf("Persisted messages: %d\n", len(msgs))
}
