//go:build integration

package integration_test

import (
	"context"
	"sync"
	"testing"
	"time"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestClient_CloseMidStream(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client := lmsdk.NewClient()
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithIncludePartialMessages(true))
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}

	if err := client.Query(ctx, promptText("Write a long essay about the history of computing.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	// Start consuming messages in background.
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		for range client.ReceiveMessages(ctx) {
		}
	}()

	// Give the stream a moment to start, then close mid-stream.
	time.Sleep(500 * time.Millisecond)

	closeStart := time.Now()
	if err := client.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	closeDuration := time.Since(closeStart)

	// Close should complete quickly (< 10s).
	if closeDuration > 10*time.Second {
		t.Fatalf("close took too long: %v", closeDuration)
	}

	wg.Wait()
}

func TestClient_ContextCancelDuringQuery(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client := lmsdk.NewClient()
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithIncludePartialMessages(true))
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	queryCtx, queryCancel := context.WithCancel(ctx)
	defer queryCancel()

	if err := client.Query(queryCtx, promptText("Write a very detailed essay about distributed systems in 50 paragraphs.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	var sawMessage bool
	var gotErr error

	for msg, err := range client.ReceiveMessages(queryCtx) {
		if err != nil {
			gotErr = err
			break
		}

		switch msg.(type) {
		case *lmsdk.StreamEvent, *lmsdk.AssistantMessage:
			if !sawMessage {
				sawMessage = true
				// Cancel the context after first message.
				queryCancel()
			}
		}
	}

	if !sawMessage {
		t.Fatal("expected at least one message before cancellation")
	}

	if gotErr == nil {
		t.Log("no error after context cancel (stream completed before cancel took effect)")
	}
}

func TestClient_RapidCloseReopen(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	for i := range 3 {
		client := lmsdk.NewClient()
		if err := client.Start(ctx, integrationOptions()...); err != nil {
			t.Fatalf("iteration %d: start: %v", i, err)
		}

		if err := client.Query(ctx, promptText("Reply with the single word: cycle.")); err != nil {
			t.Fatalf("iteration %d: query: %v", i, err)
		}

		var sawResult bool
		for msg, err := range client.ReceiveResponse(ctx) {
			if err != nil {
				t.Fatalf("iteration %d: receive: %v", i, err)
			}
			if _, ok := msg.(*lmsdk.ResultMessage); ok {
				sawResult = true
			}
		}

		if !sawResult {
			t.Fatalf("iteration %d: expected result", i)
		}

		if err := client.Close(); err != nil {
			t.Fatalf("iteration %d: close: %v", i, err)
		}
	}
}
