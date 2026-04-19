package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	); err != nil {
		fmt.Printf("client start error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	if err := client.SendToolResult(ctx, "tool-1", "{}", false); err != nil {
		var unsupported *sdk.UnsupportedControlError
		if errors.As(err, &unsupported) {
			fmt.Printf("Unsupported control as expected: %v\n", unsupported)
		} else {
			fmt.Printf("unexpected control error: %v\n", err)
		}
	}

	if _, err := sdk.StatSession(ctx, "missing-session",
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSessionStorePath(mustTempDir()),
	); err != nil {
		if errors.Is(err, sdk.ErrSessionNotFound) {
			fmt.Println("Missing session reported as expected.")
		} else {
			fmt.Printf("unexpected session error: %v\n", err)
		}
	}
}

func mustTempDir() string {
	dir, err := os.MkdirTemp("", "lm-sdk-error-handling-*")
	if err != nil {
		panic(err)
	}
	return filepath.Join(dir, "sessions.json")
}
