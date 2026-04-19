package lmsdk

import (
	"context"
	"fmt"
	"strings"
)

// ActResult summarizes the outcome of an Agent.Act call.
type ActResult struct {
	// Text is the final assistant text. Empty if the agent stopped without
	// emitting text (e.g. tool-only turns or errors).
	Text string
	// ToolCalls counts the total tool invocations during the run.
	ToolCalls int
	// Rounds counts the number of assistant messages observed.
	Rounds int
	// StopReason carries the terminal finish_reason when available.
	StopReason string
}

// Agent wraps a minimal multi-round tool loop — the `.act()` shape familiar
// to users of the official lmstudio-python and lmstudio-js SDKs. It drives
// the existing SDK tool loop until the model stops emitting tool calls.
//
// Agent.Act is a thin convenience on top of Query + WithSDKTools and is
// appropriate for single-shot agent runs. For long-lived multi-turn
// conversations, use NewClient directly.
type Agent struct {
	// Model is the target LM Studio model ID. Required.
	Model string
	// Tools are SDK tools registered as an in-process MCP server.
	Tools []Tool
	// MaxRounds caps the number of tool-use loops. Zero means use the
	// SDK default (MaxToolIterations, currently 8).
	MaxRounds int
	// ExtraOptions are passed through verbatim to Query on every Act call,
	// allowing callers to set temperature/stop/etc. once at construction.
	ExtraOptions []Option
}

// Act runs a single agent turn starting from the given prompt. The agent
// will iterate through tool calls up to MaxRounds times, returning once the
// model produces a terminal (non-tool) assistant response. Tool output is
// surfaced through the registered Tool.Execute callbacks and fed back to
// the model automatically.
func (a *Agent) Act(ctx context.Context, prompt string, opts ...Option) (*ActResult, error) {
	if strings.TrimSpace(a.Model) == "" {
		return nil, fmt.Errorf("agent: Model is required")
	}

	combined := make([]Option, 0, len(a.ExtraOptions)+len(opts)+3)
	combined = append(combined, WithModel(a.Model))
	if len(a.Tools) > 0 {
		combined = append(combined, WithSDKTools(a.Tools...))
	}
	if a.MaxRounds > 0 {
		combined = append(combined, WithMaxToolIterations(a.MaxRounds))
	}
	combined = append(combined, a.ExtraOptions...)
	combined = append(combined, opts...)

	result := &ActResult{}

	for msg, err := range Query(ctx, Text(prompt), combined...) {
		if err != nil {
			return result, err
		}
		switch m := msg.(type) {
		case *AssistantMessage:
			result.Rounds++
			for _, block := range m.Content {
				if _, ok := block.(*ToolUseBlock); ok {
					result.ToolCalls++
				}
			}
		case *ResultMessage:
			if m.Result != nil {
				result.Text = *m.Result
			}
			if m.StopReason != nil {
				result.StopReason = *m.StopReason
			}
		}
	}
	return result, nil
}
