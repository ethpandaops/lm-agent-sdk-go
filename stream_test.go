package lmsdk

import "testing"

func TestStreamHelpers(t *testing.T) {
	msg := NewUserMessage(Text("hello"))
	if msg.Message.Content.String() != "hello" {
		t.Fatalf("unexpected message: %#v", msg)
	}

	count := 0
	for range SingleMessage(Text("world")) {
		count++
	}
	if count != 1 {
		t.Fatalf("expected one message, got %d", count)
	}
}
