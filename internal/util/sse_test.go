package util

import (
	"context"
	"strings"
	"testing"
)

func TestParseSSE(t *testing.T) {
	in := strings.NewReader("data: {\"a\":1}\n\ndata: [DONE]\n\n")
	out := make(chan map[string]any, 2)
	errs := make(chan error, 2)

	ParseSSE(context.Background(), in, out, errs)

	msg, ok := <-out
	if !ok {
		t.Fatalf("out closed early")
	}
	if msg["a"].(float64) != 1 {
		t.Fatalf("unexpected msg: %#v", msg)
	}

	if _, ok := <-out; ok {
		t.Fatalf("expected out to be closed")
	}

	if err, ok := <-errs; ok && err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
