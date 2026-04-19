//go:build integration

package integration_test

import (
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestQueryStream(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	stream := lmsdk.MessagesFromContent(promptText("Reply with the single word: streamed."))
	for msg, err := range lmsdk.QueryStream(ctx, stream, integrationOptions()...) {
		if err != nil {
			t.Fatalf("query stream error: %v", err)
		}
		if _, ok := msg.(*lmsdk.ResultMessage); ok {
			return
		}
	}

	t.Fatal("expected result message")
}
