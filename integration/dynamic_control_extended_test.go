//go:build integration

package integration_test

import (
	"strings"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestDynamicControl_SetPermissionMode(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client := lmsdk.NewClient()
	if err := client.Start(ctx, integrationOptions()...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	// SetPermissionMode should succeed without error on a connected client.
	if err := client.SetPermissionMode(ctx, "plan"); err != nil {
		t.Fatalf("set permission mode: %v", err)
	}

	// Verify the client still works after mode change.
	if err := client.Query(ctx, promptText("Reply with the word: mode.")); err != nil {
		t.Fatalf("query after mode change: %v", err)
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			t.Fatalf("receive: %v", err)
		}
		if _, ok := msg.(*lmsdk.ResultMessage); ok {
			return
		}
	}

	t.Fatal("expected result after permission mode change")
}

func TestDynamicControl_SetModel(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client := lmsdk.NewClient()
	if err := client.Start(ctx, integrationOptions()...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	// SetModel should succeed.
	newModel := "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ"
	if err := client.SetModel(ctx, &newModel); err != nil {
		t.Fatalf("set model: %v", err)
	}

	// Verify the client still works after model change.
	if err := client.Query(ctx, promptText("Reply with the word: switched.")); err != nil {
		t.Fatalf("query after model change: %v", err)
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			t.Fatalf("receive: %v", err)
		}
		if _, ok := msg.(*lmsdk.ResultMessage); ok {
			return
		}
	}

	t.Fatal("expected result after model change")
}

func TestDynamicControl_Interrupt(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client := lmsdk.NewClient()
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithIncludePartialMessages(true))
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText("Write a detailed 50-paragraph essay about the history of computing.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	var sawMessage bool
	var gotErr error

	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			gotErr = err
			break
		}

		switch msg.(type) {
		case *lmsdk.StreamEvent, *lmsdk.AssistantMessage:
			if !sawMessage {
				sawMessage = true
				if err := client.Interrupt(ctx); err != nil {
					t.Fatalf("interrupt: %v", err)
				}
			}
		case *lmsdk.ResultMessage:
			// Stream completed before interrupt took effect — not an error.
			return
		}
	}

	if !sawMessage {
		t.Fatal("expected at least one message before interrupt")
	}
	if gotErr == nil {
		return // completed normally, which is fine
	}

	lower := strings.ToLower(gotErr.Error())
	if !strings.Contains(lower, "canceled") && !strings.Contains(lower, "cancelled") {
		t.Fatalf("expected cancellation error, got %v", gotErr)
	}
}
