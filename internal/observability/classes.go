package observability

import "github.com/ethpandaops/agent-sdk-observability/errclass"

// SDK-local error classes. Define only where distinguishing the class on
// dashboards is worth the extra cardinality.
const (
	ClassUnsupportedHookEvent  errclass.Class = "unsupported_hook_event"
	ClassUnsupportedHookOutput errclass.Class = "unsupported_hook_output"
	ClassExecution             errclass.Class = "execution"
)
