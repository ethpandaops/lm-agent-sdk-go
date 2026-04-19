package runtime

import (
	"errors"
	"testing"

	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/hook"
)

func TestValidateHookSupportRejectsUnsupportedEvent(t *testing.T) {
	err := ValidateHookSupport(map[hook.Event][]*hook.Matcher{
		hook.EventNotification: {},
	})
	var unsupported *sdkerrors.UnsupportedHookEventError
	if err == nil || !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedHookEventError, got %v", err)
	}
}

func TestValidateHookSupportAllowsSupportedEvents(t *testing.T) {
	err := ValidateHookSupport(map[hook.Event][]*hook.Matcher{
		hook.EventPreToolUse:        {},
		hook.EventPostToolUse:       {},
		hook.EventUserPromptSubmit:  {},
		hook.EventStop:              {},
		hook.EventPostToolUseFailure: {},
		hook.EventPermissionRequest: {},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
