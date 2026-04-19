// Package main demonstrates cancellation and graceful shutdown patterns.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

// exampleCancellation demonstrates cancelling a long-running hook callback.
func exampleCancellation() {
	fmt.Println("=== Cancellation Example ===")
	fmt.Println("Demonstrates hook callback cancellation via context.")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n[SIGNAL] Received interrupt - cancelling context")
		cancel()
	}()

	tool := sdk.NewTool("echo", "Echo text", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"echo": input["text"]}, nil
	})

	var hookCalls atomic.Int32

	longRunningHook := func(
		ctx context.Context,
		_ sdk.HookInput,
		_ *string,
		_ *sdk.HookContext,
	) (sdk.HookJSONOutput, error) {
		hookCalls.Add(1)
		fmt.Println("[HOOK] Starting long-running check...")

		for i := 1; i <= 5; i++ {
			select {
			case <-ctx.Done():
				fmt.Printf("[HOOK] Cancelled after %d seconds: %v\n", i-1, ctx.Err())
				return nil, ctx.Err()
			case <-time.After(1 * time.Second):
				fmt.Printf("[HOOK] Working... %d/5 seconds\n", i)
			}
		}

		fmt.Println("[HOOK] Completed successfully")
		return &sdk.SyncHookJSONOutput{}, nil
	}

	for msg, err := range sdk.Query(ctx,
		sdk.Text(`Call the echo tool with {"text":"hello"} and then answer with: done.`),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSDKTools(tool),
		sdk.WithHooks(map[sdk.HookEvent][]*sdk.HookMatcher{
			sdk.HookEventPreToolUse: {{
				Hooks: []sdk.HookCallback{longRunningHook},
			}},
		}),
		sdk.WithMaxTurns(4),
		sdk.WithTemperature(0),
	) {
		if err != nil {
			fmt.Printf("query ended: %v\n", err)
			break
		}
		exampleutil.DisplayMessage(msg)
	}

	fmt.Printf("Hook calls: %d\n\n", hookCalls.Load())
}

// exampleGracefulShutdown demonstrates graceful shutdown via client.Close().
func exampleGracefulShutdown() {
	fmt.Println("=== Graceful Shutdown Example ===")
	fmt.Println("Demonstrates cancelling in-flight work by closing the client.")
	fmt.Println()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	client := sdk.NewClient()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithIncludePartialMessages(true),
	); err != nil {
		fmt.Printf("start error: %v\n", err)
		return
	}

	if err := client.Query(ctx, sdk.Text("Write a very long tutorial on Go interfaces with many sections.")); err != nil {
		fmt.Printf("query error: %v\n", err)
		_ = client.Close()
		return
	}

	// Read a few messages then close
	count := 0
	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			fmt.Printf("stream ended: %v\n", err)
			break
		}
		exampleutil.DisplayMessage(msg)
		count++
		if count >= 3 {
			fmt.Println("\n[MAIN] Received 3 messages, initiating graceful shutdown...")
			break
		}
	}

	if err := client.Close(); err != nil {
		fmt.Printf("[MAIN] Close completed with: %v\n", err)
	} else {
		fmt.Println("[MAIN] Close completed successfully")
	}

	fmt.Println()
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	fmt.Println("Cancellation Examples")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()

	examples := map[string]func(){
		"cancellation":      exampleCancellation,
		"graceful_shutdown": exampleGracefulShutdown,
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <example_name>")
		fmt.Println("\nAvailable examples:")
		fmt.Println("  all               - Run all examples")
		fmt.Println("  cancellation      - Cancel a long-running hook callback")
		fmt.Println("  graceful_shutdown - Graceful shutdown of in-flight operations")
		return
	}

	exampleName := os.Args[1]

	if exampleName == "all" {
		exampleCancellation()
		fmt.Println(strings.Repeat("-", 50))
		fmt.Println()
		exampleGracefulShutdown()
	} else if fn, ok := examples[exampleName]; ok {
		fn()
	} else {
		fmt.Printf("Unknown example: %s\n", exampleName)
		os.Exit(1)
	}
}
