//go:build integration

package integration_test

import (
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestClientLifecycle(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	client := lmsdk.NewClient()
	if err := client.Start(ctx, integrationOptions()...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText("Reply with the single word: lifecycle.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			t.Fatalf("receive response: %v", err)
		}
		if _, ok := msg.(*lmsdk.ResultMessage); ok {
			return
		}
	}

	t.Fatal("expected result message")
}
