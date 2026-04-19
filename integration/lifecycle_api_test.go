//go:build integration

package integration_test

import (
	"strings"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

// TestLoadModel_RejectsEmptyModel is a read-only check: we intentionally
// send an empty request and expect LoadModel to surface the LM Studio
// server's schema error without the SDK mangling it. No actual model load
// happens so this is safe to run in CI.
func TestLoadModel_RejectsEmptyModel(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	_, err := lmsdk.LoadModel(ctx, lmsdk.LoadRequest{}, integrationOptions()...)
	if err == nil {
		t.Fatal("expected error for empty model, got nil")
	}
	// SDK-level guard catches this before the HTTP call.
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestUnloadModel_RejectsEmptyID verifies the client-side guard for
// UnloadModel (no live instance mutation).
func TestUnloadModel_RejectsEmptyID(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	err := lmsdk.UnloadModel(ctx, "", integrationOptions()...)
	if err == nil {
		t.Fatal("expected error for empty instance ID")
	}
	if !strings.Contains(err.Error(), "instance ID is required") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestDownloadStatusFor_UnknownJob asks the server about a made-up job ID.
// The server surfaces a typed error which we pass through; either an error
// or a terminal-state status is acceptable.
func TestDownloadStatusFor_UnknownJob(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	_, err := lmsdk.DownloadStatusFor(ctx, "job_does_not_exist_xyz", integrationOptions()...)
	if err == nil {
		// Some servers return a 200 with state=unknown; that's fine too.
		return
	}
	// Just make sure we're surfacing a meaningful error, not a Go panic or
	// an empty string.
	if strings.TrimSpace(err.Error()) == "" {
		t.Errorf("expected descriptive error, got empty")
	}
}
