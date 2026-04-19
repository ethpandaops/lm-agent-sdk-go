package runtime

import (
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/hook"
)

func validateHookOutputs(event hook.Event, outputs []hook.JSONOutput) error {
	for _, out := range outputs {
		if _, ok := out.(*hook.AsyncJSONOutput); ok {
			return unsupportedHookOutput(event, "async")
		}

		syncOut, ok := out.(*hook.SyncJSONOutput)
		if !ok || syncOut == nil {
			continue
		}

		// PermissionRequest-specific decision payload remains supported.
		if syncOut.SuppressOutput != nil {
			return unsupportedHookOutput(event, "suppressOutput")
		}
		if syncOut.StopReason != nil {
			return unsupportedHookOutput(event, "stopReason")
		}
		if syncOut.SystemMessage != nil {
			return unsupportedHookOutput(event, "systemMessage")
		}
		if syncOut.Continue != nil && !*syncOut.Continue {
			return unsupportedHookOutput(event, "continue")
		}
	}
	return nil
}

func unsupportedHookOutput(event hook.Event, field string) error {
	return &sdkerrors.UnsupportedHookOutputError{
		Event: string(event),
		Field: field,
	}
}
