//go:build integration

package integration_test

import (
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestSessionPersistence_ResumeSession(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	store := t.TempDir() + "/sessions.json"
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithSessionStorePath(store))

	// First query — creates session.
	result := collectResult(t, lmsdk.Query(ctx, promptText("Remember the word: elephant. Reply with just 'remembered'."), opts...))
	if result.SessionID == "" {
		t.Fatal("expected session id")
	}

	sessionID := result.SessionID

	// Wait for persistence.
	if _, err := waitForSession(ctx, sessionID, lmsdk.WithSessionStorePath(store)); err != nil {
		t.Fatalf("wait for session: %v", err)
	}

	// Resume session with second query.
	resumeOpts := append([]lmsdk.Option{}, integrationOptions()...)
	resumeOpts = append(resumeOpts,
		lmsdk.WithSessionStorePath(store),
		lmsdk.WithResume(sessionID),
	)

	result2 := collectResult(t, lmsdk.Query(ctx, promptText("What word did I ask you to remember?"), resumeOpts...))
	if result2.Result == nil {
		t.Fatal("expected result from resumed session")
	}
}

func TestSessionPersistence_MessageRetrieval(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	store := t.TempDir() + "/sessions.json"
	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithSessionStorePath(store))

	result := collectResult(t, lmsdk.Query(ctx, promptText("Reply with exactly: test-message."), opts...))
	if result.SessionID == "" {
		t.Fatal("expected session id")
	}

	// Wait for persistence.
	stat, err := waitForSession(ctx, result.SessionID, lmsdk.WithSessionStorePath(store))
	if err != nil {
		t.Fatalf("wait for session: %v", err)
	}

	// Verify message count.
	if stat.MessageCount == 0 {
		t.Fatalf("expected messages in session, got %+v", stat)
	}
	if stat.UserTurns == 0 {
		t.Fatalf("expected user turns > 0, got %+v", stat)
	}

	// Verify message retrieval.
	msgs, err := lmsdk.GetSessionMessages(ctx, result.SessionID, lmsdk.WithSessionStorePath(store))
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected persisted messages")
	}

	// Verify we have both user and assistant messages.
	var sawUser, sawAssistant bool
	for _, msg := range msgs {
		switch msg.(type) {
		case *lmsdk.UserMessage:
			sawUser = true
		case *lmsdk.AssistantMessage:
			sawAssistant = true
		}
	}

	if !sawUser {
		t.Fatal("expected user message in session")
	}
	if !sawAssistant {
		t.Fatal("expected assistant message in session")
	}
}

func TestSessionNotFound(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	store := t.TempDir() + "/sessions.json"
	_, err := lmsdk.StatSession(ctx, "nonexistent-session-id", lmsdk.WithSessionStorePath(store))

	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}
