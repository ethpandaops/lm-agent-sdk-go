package hook

import (
	"context"
	"time"
)

// Dispatcher runs hook callbacks for configured events.
type Dispatcher struct {
	hooks map[Event][]*Matcher
}

// NewDispatcher creates a dispatcher from hook config.
func NewDispatcher(hooks map[Event][]*Matcher) *Dispatcher {
	if hooks == nil {
		hooks = map[Event][]*Matcher{}
	}
	return &Dispatcher{hooks: hooks}
}

// Run executes matching callbacks for an event and tool name.
func (d *Dispatcher) Run(
	ctx context.Context,
	event Event,
	toolName string,
	input Input,
	toolUseID *string,
) ([]JSONOutput, error) {
	matchers := d.hooks[event]
	if len(matchers) == 0 {
		return nil, nil
	}

	outputs := make([]JSONOutput, 0, len(matchers))
	hookCtx := &Context{}
	for _, matcher := range matchers {
		if matcher == nil || !matcher.Matches(toolName) {
			continue
		}

		matcherCtx := ctx
		cancel := func() {}
		if matcher.Timeout != nil && *matcher.Timeout > 0 {
			timeout := time.Duration(*matcher.Timeout * float64(time.Second))
			matcherCtx, cancel = context.WithTimeout(ctx, timeout)
		}

		for _, cb := range matcher.Hooks {
			out, err := runHookCallback(matcherCtx, cb, input, toolUseID, hookCtx)
			if err != nil {
				cancel()
				return outputs, err
			}
			outputs = append(outputs, out)
		}
		cancel()
	}
	return outputs, nil
}

func runHookCallback(
	ctx context.Context,
	cb Callback,
	input Input,
	toolUseID *string,
	hookCtx *Context,
) (JSONOutput, error) {
	type result struct {
		out JSONOutput
		err error
	}

	done := make(chan result, 1)
	go func() {
		out, err := cb(ctx, input, toolUseID, hookCtx)
		done <- result{out: out, err: err}
	}()

	select {
	case r := <-done:
		return r.out, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
