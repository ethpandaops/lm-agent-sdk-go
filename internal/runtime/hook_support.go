package runtime

import (
	"github.com/ethpandaops/lm-agent-sdk-go/internal/hook"
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
)

var supportedHookEvents = map[hook.Event]struct{}{
	hook.EventPreToolUse:         {},
	hook.EventPostToolUse:        {},
	hook.EventUserPromptSubmit:   {},
	hook.EventStop:               {},
	hook.EventPostToolUseFailure: {},
	hook.EventPermissionRequest:  {},
}

// ValidateHookSupport ensures configured hook events are implemented by this backend.
func ValidateHookSupport(hooks map[hook.Event][]*hook.Matcher) error {
	for ev := range hooks {
		if _, ok := supportedHookEvents[ev]; ok {
			continue
		}
		return &sdkerrors.UnsupportedHookEventError{Event: string(ev)}
	}
	return nil
}
