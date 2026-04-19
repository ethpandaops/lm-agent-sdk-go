//go:build integration

package integration_test

import (
	"context"
	"sync/atomic"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestOnUserInput(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var invoked atomic.Bool
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithOnUserInput(func(_ context.Context, req *lmsdk.UserInputRequest) (*lmsdk.UserInputResponse, error) {
			invoked.Store(true)
			return &lmsdk.UserInputResponse{
				Answers: map[string]*lmsdk.UserInputAnswer{
					req.Questions[0].ID: {Answers: []string{"yes"}},
				},
			}, nil
		}),
		lmsdk.WithToolChoice("auto"),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText(`Call the stdio tool to ask whether you should continue.`)); err != nil {
		t.Fatalf("query: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveMessages(ctx) {
		}
	}()

	if err := waitForCondition(ctx, func() bool { return invoked.Load() }); err != nil {
		t.Fatalf("wait for user input callback: %v", err)
	}
	_ = client.Interrupt(ctx)
	<-done

	if !invoked.Load() {
		t.Fatal("expected OnUserInput callback to run")
	}
}
