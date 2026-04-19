package lmsdk

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStatSession_NotFound(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := t.TempDir() + "/sessions.json"
	_, err := StatSession(ctx, "missing", WithSessionStorePath(store))
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}
