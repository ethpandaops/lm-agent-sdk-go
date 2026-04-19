package runtime

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	agenterrclass "github.com/ethpandaops/agent-sdk-observability/errclass"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/hook"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/message"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/observability"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/permission"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/session"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/tools"
	upstreamgenai "go.opentelemetry.io/otel/semconv/v1.40.0/genaiconv"
)

type pendingToolCall struct {
	ID   string
	Name string
	Args strings.Builder
}

func attachAuditEnvelope(msg message.Message, eventType, subtype string, payload any) {
	audit, err := message.NewAuditEnvelope(eventType, subtype, payload)
	if err != nil {
		return
	}

	switch typed := msg.(type) {
	case *message.UserMessage:
		typed.Audit = audit
	case *message.AssistantMessage:
		typed.Audit = audit
	case *message.SystemMessage:
		typed.Audit = audit
	case *message.ResultMessage:
		typed.Audit = audit
	case *message.StreamEvent:
		typed.Audit = audit
	}
}

func attachRawAuditEnvelope(
	msg message.Message, eventType, subtype string, payload map[string]any,
) {
	if len(payload) == 0 {
		return
	}

	attachAuditEnvelope(msg, eventType, subtype, payload)
}

// QueryRunner executes prompt/query flows over the configured transport.
type QueryRunner struct {
	opts      *config.Options
	transport config.Transport
	sessions  *session.Manager
	hooks     *hook.Dispatcher
	registry  *tools.Registry
	executor  *tools.Executor
	obs       *observability.Observer
}

// NewQueryRunner creates a QueryRunner.
func NewQueryRunner(
	opts *config.Options,
	sessions *session.Manager,
	obs *observability.Observer,
) *QueryRunner {
	if sessions == nil {
		sessions = session.NewManager()
	}
	if obs == nil {
		obs = observability.Noop()
	}
	opts.ApplyDefaults()
	registry := tools.NewRegistry(opts)
	return &QueryRunner{
		opts:      opts,
		transport: opts.Transport,
		sessions:  sessions,
		hooks:     NewHookDispatcher(opts),
		registry:  registry,
		executor:  tools.NewExecutor(opts, registry),
		obs:       obs,
	}
}

// MCPServerStatuses returns MCP server readiness discovered during registry init.
func (r *QueryRunner) MCPServerStatuses() map[string]mcp.ServerStatus {
	if r == nil || r.registry == nil {
		return map[string]mcp.ServerStatus{}
	}
	return r.registry.ServerStatuses()
}

// SetPermissionMode updates the active permission mode used by tool execution.
func (r *QueryRunner) SetPermissionMode(mode string) {
	if r == nil {
		return
	}
	if r.opts != nil {
		r.opts.PermissionMode = mode
	}
	if r.executor != nil && mode != "" {
		r.executor.SetMode(permission.Mode(mode))
	}
}

// NewHookDispatcher is split for testability.
func NewHookDispatcher(opts *config.Options) *hook.Dispatcher {
	if opts == nil {
		return hook.NewDispatcher(nil)
	}
	return hook.NewDispatcher(opts.Hooks)
}

// runHookWithObs wraps a hook dispatch call with observability instrumentation,
// creating a span and recording the hook execution duration.
func (r *QueryRunner) runHookWithObs(
	ctx context.Context,
	event hook.Event,
	toolName string,
	input hook.Input,
	toolUseID *string,
) ([]hook.JSONOutput, error) {
	hookCtx, hookSpan := r.obs.StartHookSpan(ctx, string(event))
	start := time.Now()
	outs, err := r.hooks.Run(hookCtx, event, toolName, input, toolUseID)
	outcome := "ok"
	if err != nil {
		outcome = "error"
		hookSpan.RecordError(err)
	} else {
		hookSpan.SetAttributes(observability.Outcome(outcome))
	}
	hookSpan.End()
	r.obs.RecordHookDuration(hookCtx, time.Since(start).Seconds(), string(event), outcome)

	return outs, err
}

// observeMessage calls the configured SessionMetricsRecorder for message-level
// observability (TTFT, token usage, operation duration, span enrichment).
func (r *QueryRunner) observeMessage(ctx context.Context, msg message.Message) {
	if r.opts != nil && r.opts.MetricsRecorder != nil {
		r.opts.MetricsRecorder.Observe(ctx, msg)
	}
}

// RunPrompt runs a simple user-content turn and streams message outputs.
func (r *QueryRunner) RunPrompt(
	ctx context.Context,
	sessionID string,
	content message.UserMessageContent,
) (<-chan message.Message, <-chan error) {
	msg := message.StreamingMessage{
		Type: "user",
		Message: message.StreamingMessageContent{
			Role:    "user",
			Content: content,
		},
		SessionID: sessionID,
	}
	return r.RunMessages(ctx, sessionID, []message.StreamingMessage{msg})
}

// RunMessageIterator incrementally consumes streaming input and processes each
// user message against shared session state.
func (r *QueryRunner) RunMessageIterator(
	ctx context.Context,
	sessionID string,
	inputs iter.Seq[message.StreamingMessage],
) (<-chan message.Message, <-chan error) {
	out := make(chan message.Message, 64)
	errs := make(chan error, 8)

	go func() {
		defer close(out)
		defer close(errs)

		sid := sessionID
		if sid == "" {
			sid = "default"
		}
		inputCh := make(chan message.StreamingMessage, 16)
		var ingestWG sync.WaitGroup
		ingestWG.Add(1)
		go func() {
			defer ingestWG.Done()
			defer close(inputCh)
			for in := range inputs {
				select {
				case inputCh <- in:
				case <-ctx.Done():
					return
				}
			}
		}()
		defer ingestWG.Wait()

		for {
			var in message.StreamingMessage
			var ok bool
			select {
			case in, ok = <-inputCh:
				if !ok {
					return
				}
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}
			runSID := sid
			if in.SessionID != "" {
				runSID = in.SessionID
				sid = runSID
			}

			msgs, runErrs := r.RunMessages(ctx, runSID, []message.StreamingMessage{in})
			msgClosed := false
			errClosed := false

			for !msgClosed || !errClosed {
				select {
				case msg, ok := <-msgs:
					if !ok {
						msgClosed = true
						continue
					}
					select {
					case out <- msg:
					case <-ctx.Done():
						errs <- ctx.Err()
						return
					}
				case err, ok := <-runErrs:
					if !ok {
						errClosed = true
						continue
					}
					if err != nil {
						select {
						case errs <- err:
						case <-ctx.Done():
						}
						return
					}
				case <-ctx.Done():
					errs <- ctx.Err()
					return
				}
			}
		}
	}()

	return out, errs
}

// RunMessages runs a conversation turn from one or more user messages.
func (r *QueryRunner) RunMessages(
	ctx context.Context,
	sessionID string,
	inputs []message.StreamingMessage,
) (<-chan message.Message, <-chan error) {
	out := make(chan message.Message, 64)
	errs := make(chan error, 8)

	go func() {
		defer close(out)
		defer close(errs)

		if r.transport == nil {
			errs <- fmt.Errorf("transport is nil")
			return
		}
		if err := r.transport.Start(ctx); err != nil {
			errs <- err
			return
		}

		if sessionID == "" {
			sessionID = "default"
		}
		s := r.sessions.GetOrCreate(sessionID)
		runStarted := time.Now()

		// Notify the metrics recorder that a new query is starting (TTFT baseline).
		if notifier, ok := r.opts.MetricsRecorder.(config.QueryLifecycleNotifier); ok {
			notifier.MarkQueryStart()
		}

		// Start query span for the entire conversation turn.
		model := pickModel(r.opts)
		ctx, querySpan := r.obs.StartQuerySpan(ctx, upstreamgenai.OperationNameChat, model, sessionID)
		hasRecorder := r.opts.MetricsRecorder != nil
		var errType string
		defer func() {
			// When a MetricsRecorder is configured, operation duration and
			// error classification are recorded from the ResultMessage via
			// the recorder's Observe method. Otherwise, fall back to direct
			// Observer recording.
			if !hasRecorder {
				duration := time.Since(runStarted).Seconds()
				r.obs.RecordOperationDuration(ctx, duration, upstreamgenai.OperationNameChat, model, agenterrclass.Class(errType))
			}
			if errType != "" {
				querySpan.MarkError(agenterrclass.Class(errType))
			}
			// Unset span status implies success — no explicit Ok needed.
			querySpan.End()
		}()

		history := session.Clone(s.Messages)
		if len(history) == 0 {
			if sys := effectiveSystemPrompt(r.opts); sys != "" {
				systemMsg := &message.SystemMessage{
					Type:    "system",
					Subtype: "init",
					Data: map[string]any{
						"prompt": sys,
					},
				}
				attachAuditEnvelope(systemMsg, "system", "init", map[string]any{
					"type":    "system",
					"subtype": "init",
					"data":    systemMsg.Data,
				})
				select {
				case out <- systemMsg:
				case <-ctx.Done():
					errType = "cancelled"
					errs <- ctx.Err()
					return
				}
				history = append(history, map[string]any{
					"role":    "system",
					"content": sys,
				})
			}
		}

		for _, in := range inputs {
			if in.Message.Role == "" {
				in.Message.Role = "user"
			}
			userID := fmt.Sprintf("%s-u%d", sessionID, s.UserTurns+1)
			s.UserTurns++

			um := &message.UserMessage{
				Type:    "user",
				Content: in.Message.Content,
				UUID:    &userID,
			}
			attachAuditEnvelope(um, "user", "", um)
			select {
			case out <- um:
			case <-ctx.Done():
				errType = "cancelled"
				errs <- ctx.Err()
				return
			}

			hookOuts, err := r.runHookWithObs(ctx, hook.EventUserPromptSubmit, "", &hook.UserPromptSubmitInput{
				BaseInput:     baseInput(sessionID, r.opts),
				HookEventName: string(hook.EventUserPromptSubmit),
				Prompt:        in.Message.Content.String(),
			}, nil)
			if err != nil {
				errType = "hook_error"
				errs <- err
				return
			}
			if err := validateHookOutputs(hook.EventUserPromptSubmit, hookOuts); err != nil {
				errType = "hook_error"
				errs <- err
				return
			}

			history = append(history, map[string]any{
				"role":    in.Message.Role,
				"content": historyUserContent(in.Message.Content),
			})

			if r.opts.EnableFileCheckpointing {
				r.sessions.SetState(sessionID, history, s.UserTurns)
				r.sessions.Snapshot(sessionID, userID)
				if r.opts.Cwd != "" {
					if err := r.sessions.SnapshotFiles(sessionID, userID, r.opts.Cwd); err != nil {
						errType = "checkpoint_error"
						errs <- err
						return
					}
				}
			}
		}

		maxTurns := r.opts.MaxTurns
		if maxTurns <= 0 {
			maxTurns = r.opts.MaxToolIterations
		}
		if maxTurns <= 0 {
			maxTurns = 8
		}
		totalCost := 0.0

		for turn := 0; turn < maxTurns; turn++ {
			models := []string{pickModel(r.opts)}
			if r.opts != nil && r.opts.FallbackModel != "" && r.opts.FallbackModel != models[0] {
				models = append(models, r.opts.FallbackModel)
			}

			reqModel := models[0]
			calls := map[int]*pendingToolCall{}
			assistantTextStr := ""
			assistantReasoningStr := ""
			assistantImages := []message.ImageBlock{}
			var turnUsage *message.Usage
			var turnCost *float64
			var terminalEvent map[string]any
			var runErr error
			turnTotalCost := 0.0
			turnCostSeen := false

			for i, model := range models {
				reqModel = model
				tools := r.registry.OpenAITools()
				toolChoice := requestToolChoice(r.opts)
				// WithForceTool applies only to the opening turn so the model
				// can follow up normally with prose once the tool has run.
				// Later turns fall back to the user-supplied tool_choice (or
				// default "auto") and the full tools set.
				if turn == 0 {
					if forced := strings.TrimSpace(r.opts.ForcedTool); forced != "" {
						tools = filterToolsByName(tools, forced)
						toolChoice = "required"
					}
				}
				req := &config.ChatRequest{
					Model:              model,
					Messages:           history,
					Tools:              tools,
					Stream:             true,
					ToolChoice:         toolChoice,
					MaxTokens:          requestMaxTokens(r.opts),
					Temperature:        requestTemperature(r.opts),
					TopP:               requestTopP(r.opts),
					TopK:               requestTopK(r.opts),
					PresencePenalty:    requestPresencePenalty(r.opts),
					FrequencyPenalty:   requestFrequencyPenalty(r.opts),
					Seed:               requestSeed(r.opts),
					Stop:               requestStop(r.opts),
					Logprobs:           requestLogprobs(r.opts),
					TopLogprobs:        requestTopLogprobs(r.opts),
					ParallelToolCalls:  requestParallelToolCalls(r.opts),
					ResponseFormat:     r.opts.OutputFormat,
					Reasoning:          requestReasoning(r.opts),
					User:               requestUser(r.opts),
					MaxToolCalls:       requestMaxToolCalls(r.opts),
					StreamIncludeUsage: requestStreamIncludeUsage(r.opts),
					Extra:              requestExtra(r.opts),
					TTL:                requestTTL(r.opts),
					DraftModel:         requestDraftModel(r.opts),
					MinP:               requestMinP(r.opts),
					RepeatPenalty:      requestRepeatPenalty(r.opts),
				}
				var emitted bool
				assistantTextStr, assistantReasoningStr, assistantImages, calls, _, turnUsage, turnCost, terminalEvent, runErr, emitted = r.runStream(ctx, sessionID, req, out, errs)
				if turnCost != nil {
					turnTotalCost += *turnCost
					turnCostSeen = true
				}
				if runErr == nil {
					break
				}
				// If stream already emitted output, do not retry with fallback to avoid
				// duplicate/partial mixed turn output.
				if emitted {
					errType = "transport_error"
					errs <- runErr
					return
				}
				if i == len(models)-1 {
					errType = "transport_error"
					errs <- runErr
					return
				}
			}

			// Update model to reflect the actual model used (may differ
			// from initial pick after fallback).
			model = reqModel

			// Record token usage from the completed stream. When a
			// MetricsRecorder is configured, token recording is handled
			// by the recorder's observeResult from the ResultMessage.
			if !hasRecorder && turnUsage != nil {
				r.obs.RecordTokenUsage(ctx, int64(turnUsage.InputTokens),
					upstreamgenai.TokenTypeInput, upstreamgenai.OperationNameChat, reqModel)
				r.obs.RecordTokenUsage(ctx, int64(turnUsage.OutputTokens),
					upstreamgenai.TokenTypeOutput, upstreamgenai.OperationNameChat, reqModel)
				if turnUsage.ReasoningOutputTokens > 0 {
					r.obs.RecordTokenUsage(ctx, int64(turnUsage.ReasoningOutputTokens),
						upstreamgenai.TokenTypeAttr("thinking"), upstreamgenai.OperationNameChat, reqModel)
				}
			}

			if turnCostSeen {
				totalCost += turnTotalCost
			}
			if r.opts != nil && r.opts.MaxBudgetUSD != nil && turnCostSeen && totalCost > *r.opts.MaxBudgetUSD {
				errType = "budget_exceeded"
				msg := fmt.Sprintf("max budget exceeded: spent %.6f > budget %.6f", totalCost, *r.opts.MaxBudgetUSD)
				res := &message.ResultMessage{
					Type:          "result",
					Subtype:       "error_max_budget_usd",
					NumTurns:      turn + 1,
					SessionID:     sessionID,
					IsError:       true,
					DurationMs:    elapsedMs(runStarted),
					DurationAPIMs: elapsedMs(runStarted),
					TotalCostUSD:  ptrFloat(totalCost),
					Usage:         turnUsage,
					Result:        &msg,
					StopReason:    ptrString("max_budget"),
				}
				if terminalEvent != nil {
					attachRawAuditEnvelope(
						res, "result", res.Subtype, terminalEvent,
					)
				} else {
					attachAuditEnvelope(
						res, "result", res.Subtype, res,
					)
				}
				r.observeMessage(ctx, res)

				select {
				case out <- res:
				case <-ctx.Done():
					errs <- ctx.Err()
				}
				return
			}

			// Build assistant message for history.
			assistantHistory := map[string]any{"role": "assistant"}
			if assistantTextStr != "" {
				assistantHistory["content"] = assistantTextStr
			} else {
				assistantHistory["content"] = nil
			}
			if len(assistantImages) > 0 {
				assistantHistory["images"] = encodeHistoryImages(assistantImages)
			}
			// Some reasoning models (DeepSeek R1 derivatives, certain Qwen3
			// variants) 400 on follow-up requests when the prior assistant
			// turn's reasoning_content isn't echoed back. Preserve it under
			// both modern (`reasoning`) and legacy (`reasoning_content`)
			// field names — the server uses whichever one it understands.
			if assistantReasoningStr != "" {
				assistantHistory["reasoning"] = assistantReasoningStr
				assistantHistory["reasoning_content"] = assistantReasoningStr
			}
			if !r.opts.IncludePartialMessages {
				assistantBlocks := assistantContentBlocks(assistantTextStr, assistantReasoningStr, assistantImages)
				if len(assistantBlocks) > 0 {
					am := &message.AssistantMessage{
						Type:    "assistant",
						Model:   reqModel,
						Content: assistantBlocks,
					}
					attachAuditEnvelope(am, "assistant", "final_text", am)
					r.observeMessage(ctx, am)

					select {
					case out <- am:
					case <-ctx.Done():
						errType = "cancelled"
						errs <- ctx.Err()
						return
					}
				}
			}

			orderedCalls := orderCalls(calls)
			if len(orderedCalls) > 0 {
				toolCalls := make([]map[string]any, 0, len(orderedCalls))
				toolResultsHistory := make([]map[string]any, 0, len(orderedCalls))
				for _, c := range orderedCalls {
					argsMap := map[string]any{}
					if strings.TrimSpace(c.Args.String()) != "" {
						_ = json.Unmarshal([]byte(c.Args.String()), &argsMap)
					}

					assistantToolMsg := &message.AssistantMessage{
						Type:  "assistant",
						Model: reqModel,
						Content: []message.ContentBlock{
							&message.ToolUseBlock{
								Type:  message.BlockTypeToolUse,
								ID:    c.ID,
								Name:  c.Name,
								Input: argsMap,
							},
						},
					}
					attachAuditEnvelope(assistantToolMsg, "assistant", "tool_use", assistantToolMsg)
					r.observeMessage(ctx, assistantToolMsg)

					select {
					case out <- assistantToolMsg:
					case <-ctx.Done():
						errType = "cancelled"
						errs <- ctx.Err()
						return
					}

					toolUseID := c.ID
					hookOuts, err := r.runHookWithObs(ctx, hook.EventPermissionRequest, c.Name, &hook.PermissionRequestInput{
						BaseInput:             baseInput(sessionID, r.opts),
						HookEventName:         string(hook.EventPermissionRequest),
						ToolName:              c.Name,
						ToolInput:             argsMap,
						PermissionSuggestions: nil,
					}, &toolUseID)
					if err != nil {
						errType = "hook_error"
						errs <- err
						return
					}
					if err := validateHookOutputs(hook.EventPermissionRequest, hookOuts); err != nil {
						errType = "hook_error"
						errs <- err
						return
					}
					decision := permissionDecisionFromHookOutputs(hookOuts, argsMap)
					if len(decision.updatedPermissions) > 0 {
						r.executor.ApplyPermissionUpdates(decision.updatedPermissions)
					}
					if decision.updatedInput != nil {
						argsMap = decision.updatedInput
					}
					if decision.deny {
						denyErr := &sdkerrors.ToolPermissionDeniedError{
							ToolName:  c.Name,
							Message:   decision.message,
							Interrupt: decision.interrupt,
						}
						toolCtx, toolSpan := r.obs.StartToolSpan(ctx, c.Name, c.ID)
						toolSpan.RecordError(denyErr)
						toolSpan.SetAttributes(observability.Outcome("denied"))
						toolSpan.End()
						r.obs.RecordToolCall(toolCtx, c.Name, "denied")
						v := denyErr.Interrupt
						postFailureOuts, _ := r.runHookWithObs(ctx, hook.EventPostToolUseFailure, c.Name, &hook.PostToolUseFailureInput{
							BaseInput:     baseInput(sessionID, r.opts),
							HookEventName: string(hook.EventPostToolUseFailure),
							ToolName:      c.Name,
							ToolInput:     argsMap,
							ToolUseID:     c.ID,
							Error:         denyErr.Error(),
							IsInterrupt:   &v,
						}, &toolUseID)
						if err := validateHookOutputs(hook.EventPostToolUseFailure, postFailureOuts); err != nil {
							errType = "hook_error"
							errs <- err
							return
						}
						if denyErr.Interrupt {
							errType = "permission_denied"
							msg := denyErr.Error()
							res := &message.ResultMessage{
								Type:       "result",
								Subtype:    "error",
								NumTurns:   turn + 1,
								SessionID:  sessionID,
								IsError:    true,
								DurationMs: elapsedMs(runStarted),
								Result:     &msg,
								StopReason: ptrString("interrupted"),
							}
							attachAuditEnvelope(res, "result", res.Subtype, res)
							select {
							case out <- res:
							case <-ctx.Done():
								errs <- ctx.Err()
							}
							return
						}
						errType = "permission_denied"
						errs <- denyErr
						return
					}

					preToolOuts, err := r.runHookWithObs(ctx, hook.EventPreToolUse, c.Name, &hook.PreToolUseInput{
						BaseInput:     baseInput(sessionID, r.opts),
						HookEventName: string(hook.EventPreToolUse),
						ToolName:      c.Name,
						ToolInput:     argsMap,
						ToolUseID:     c.ID,
					}, &toolUseID)
					if err != nil {
						errType = "hook_error"
						errs <- err
						return
					}
					if err := validateHookOutputs(hook.EventPreToolUse, preToolOuts); err != nil {
						errType = "hook_error"
						errs <- err
						return
					}

					toolStart := time.Now()
					toolCtx, toolSpan := r.obs.StartToolSpan(ctx, c.Name, c.ID)
					toolOut, err := r.executor.ExecuteWithSuggestions(toolCtx, c.Name, argsMap, decision.suggestions)
					toolDuration := time.Since(toolStart).Seconds()
					r.obs.RecordToolCallDuration(toolCtx, toolDuration, c.Name)
					if err != nil {
						outcome := "error"
						var denyErr *sdkerrors.ToolPermissionDeniedError
						var isInterrupt *bool
						if stderrors.As(err, &denyErr) {
							v := denyErr.Interrupt
							isInterrupt = &v
							outcome = "denied"
						}
						toolSpan.RecordError(err)
						toolSpan.SetAttributes(observability.Outcome(outcome))
						toolSpan.End()
						r.obs.RecordToolCall(toolCtx, c.Name, outcome)
						postFailureOuts, _ := r.runHookWithObs(ctx, hook.EventPostToolUseFailure, c.Name, &hook.PostToolUseFailureInput{
							BaseInput:     baseInput(sessionID, r.opts),
							HookEventName: string(hook.EventPostToolUseFailure),
							ToolName:      c.Name,
							ToolInput:     argsMap,
							ToolUseID:     c.ID,
							Error:         err.Error(),
							IsInterrupt:   isInterrupt,
						}, &toolUseID)
						if hookErr := validateHookOutputs(hook.EventPostToolUseFailure, postFailureOuts); hookErr != nil {
							errType = "hook_error"
							errs <- hookErr
							return
						}
						if denyErr != nil && denyErr.Interrupt {
							errType = "permission_denied"
							msg := denyErr.Error()
							res := &message.ResultMessage{
								Type:       "result",
								Subtype:    "error",
								NumTurns:   turn + 1,
								SessionID:  sessionID,
								IsError:    true,
								DurationMs: elapsedMs(runStarted),
								Result:     &msg,
								StopReason: ptrString("interrupted"),
							}
							attachAuditEnvelope(res, "result", res.Subtype, res)
							select {
							case out <- res:
							case <-ctx.Done():
								errs <- ctx.Err()
							}
							return
						}
						if denyErr != nil {
							errType = "permission_denied"
							errs <- err
							return
						}
						// Non-permission tool errors are recoverable:
						// return the error as a tool result so the model
						// can see its mistake and retry.
						toolOut = "Error: " + err.Error()
					} else {
						// Unset span status implies success.
						toolSpan.End()
						r.obs.RecordToolCall(toolCtx, c.Name, "ok")
					}

					postToolOuts, err := r.runHookWithObs(ctx, hook.EventPostToolUse, c.Name, &hook.PostToolUseInput{
						BaseInput:     baseInput(sessionID, r.opts),
						HookEventName: string(hook.EventPostToolUse),
						ToolName:      c.Name,
						ToolInput:     argsMap,
						ToolUseID:     c.ID,
						ToolResponse:  toolOut,
					}, &toolUseID)
					if err != nil {
						errType = "hook_error"
						errs <- err
						return
					}
					if err := validateHookOutputs(hook.EventPostToolUse, postToolOuts); err != nil {
						errType = "hook_error"
						errs <- err
						return
					}

					toolResMsg := &message.AssistantMessage{
						Type:  "assistant",
						Model: reqModel,
						Content: []message.ContentBlock{
							&message.ToolResultBlock{
								Type:      message.BlockTypeToolResult,
								ToolUseID: c.ID,
								Content: []message.ContentBlock{
									&message.TextBlock{Type: message.BlockTypeText, Text: toolOut},
								},
							},
						},
					}
					attachAuditEnvelope(toolResMsg, "assistant", "tool_result", toolResMsg)
					select {
					case out <- toolResMsg:
					case <-ctx.Done():
						errs <- ctx.Err()
						return
					}

					toolCalls = append(toolCalls, map[string]any{
						"id":   c.ID,
						"type": "function",
						"function": map[string]any{
							"name":      c.Name,
							"arguments": c.Args.String(),
						},
					})
					toolResultsHistory = append(toolResultsHistory, map[string]any{
						"role":         "tool",
						"tool_call_id": c.ID,
						"content":      toolOut,
					})
				}
				assistantHistory["tool_calls"] = toolCalls
				history = append(history, assistantHistory)
				history = append(history, toolResultsHistory...)

				// Tool calls mean continue conversation with another request.
				continue
			}

			history = append(history, assistantHistory)

			resText := strings.TrimSpace(assistantTextStr)
			var resultText *string
			if resText != "" {
				resultText = ptrString(resText)
			}
			structured := parseStructuredOutput(resText, assistantReasoningStr, r.opts)
			result := &message.ResultMessage{
				Type:             "result",
				Subtype:          "success",
				NumTurns:         turn + 1,
				SessionID:        sessionID,
				IsError:          false,
				DurationMs:       elapsedMs(runStarted),
				DurationAPIMs:    elapsedMs(runStarted),
				TotalCostUSD:     totalCostPtr(totalCost, turnCostSeen),
				Usage:            turnUsage,
				Result:           resultText,
				StopReason:       ptrString("end_turn"),
				StructuredOutput: structured,
			}
			if terminalEvent != nil {
				attachRawAuditEnvelope(
					result, "result", result.Subtype, terminalEvent,
				)
			} else {
				attachAuditEnvelope(
					result, "result", result.Subtype, result,
				)
			}
			r.observeMessage(ctx, result)

			select {
			case out <- result:
			case <-ctx.Done():
				errs <- ctx.Err()
				return
			}

			r.sessions.SetState(sessionID, history, s.UserTurns)
			stopOuts, _ := r.runHookWithObs(ctx, hook.EventStop, "", &hook.StopInput{
				BaseInput:     baseInput(sessionID, r.opts),
				HookEventName: string(hook.EventStop),
			}, nil)
			if err := validateHookOutputs(hook.EventStop, stopOuts); err != nil {
				errType = "hook_error"
				errs <- err
				return
			}
			return
		}

		errType = "max_turns"
		errs <- fmt.Errorf("max turns reached without terminal response")
	}()

	return out, errs
}

func ptrString(s string) *string  { return &s }
func ptrFloat(v float64) *float64 { return &v }
func totalCostPtr(total float64, seen bool) *float64 {
	if !seen {
		return nil
	}
	return ptrFloat(total)
}

func elapsedMs(start time.Time) int {
	ms := int(time.Since(start).Milliseconds())
	if ms <= 0 {
		return 1
	}
	return ms
}

func pickModel(opts *config.Options) string {
	if opts != nil && opts.Model != "" {
		return opts.Model
	}
	return ""
}

func baseInput(sessionID string, opts *config.Options) hook.BaseInput {
	var mode *string
	if opts != nil && opts.PermissionMode != "" {
		m := opts.PermissionMode
		mode = &m
	}
	cwd := ""
	if opts != nil {
		cwd = opts.Cwd
	}
	return hook.BaseInput{SessionID: sessionID, Cwd: cwd, PermissionMode: mode}
}

func requestUser(opts *config.Options) string {
	if opts == nil {
		return ""
	}
	return strings.TrimSpace(opts.User)
}

func requestToolChoice(opts *config.Options) any {
	if opts == nil {
		return nil
	}
	return opts.ToolChoice
}

// filterToolsByName returns only the OpenAI-tool entries matching `name` (by
// function.name). Used by WithForceTool to narrow the tools[] slice so that
// `tool_choice: "required"` effectively forces a specific function on
// LM Studio, which rejects the OpenAI object form of tool_choice.
func filterToolsByName(tools []map[string]any, name string) []map[string]any {
	if name == "" || len(tools) == 0 {
		return tools
	}
	out := make([]map[string]any, 0, 1)
	for _, t := range tools {
		fn, ok := t["function"].(map[string]any)
		if !ok {
			continue
		}
		if n, _ := fn["name"].(string); n == name {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		// Unknown force name — fall back to the full set rather than
		// sending an empty tools[] (which would make "required" unsatisfiable).
		return tools
	}
	return out
}

func requestMaxTokens(opts *config.Options) *int {
	if opts == nil || opts.MaxTokens == nil {
		return nil
	}
	v := *opts.MaxTokens
	return &v
}

func requestTemperature(opts *config.Options) *float64 {
	if opts == nil || opts.Temperature == nil {
		return nil
	}
	v := *opts.Temperature
	return &v
}

func requestTopP(opts *config.Options) *float64 {
	if opts == nil || opts.TopP == nil {
		return nil
	}
	v := *opts.TopP
	return &v
}

func requestTopK(opts *config.Options) *float64 {
	if opts == nil || opts.TopK == nil {
		return nil
	}
	v := *opts.TopK
	return &v
}

func requestMinP(opts *config.Options) *float64 {
	if opts == nil || opts.MinP == nil {
		return nil
	}
	v := *opts.MinP
	return &v
}

func requestRepeatPenalty(opts *config.Options) *float64 {
	if opts == nil || opts.RepeatPenalty == nil {
		return nil
	}
	v := *opts.RepeatPenalty
	return &v
}

func requestPresencePenalty(opts *config.Options) *float64 {
	if opts == nil || opts.PresencePenalty == nil {
		return nil
	}
	v := *opts.PresencePenalty
	return &v
}

func requestFrequencyPenalty(opts *config.Options) *float64 {
	if opts == nil || opts.FrequencyPenalty == nil {
		return nil
	}
	v := *opts.FrequencyPenalty
	return &v
}

func requestSeed(opts *config.Options) *int64 {
	if opts == nil || opts.Seed == nil {
		return nil
	}
	v := *opts.Seed
	return &v
}

func requestStop(opts *config.Options) []string {
	if opts == nil || len(opts.Stop) == 0 {
		return nil
	}
	return append([]string(nil), opts.Stop...)
}

func requestLogprobs(opts *config.Options) *bool {
	if opts == nil || opts.Logprobs == nil {
		return nil
	}
	v := *opts.Logprobs
	return &v
}

func requestTopLogprobs(opts *config.Options) *int {
	if opts == nil || opts.TopLogprobs == nil {
		return nil
	}
	v := *opts.TopLogprobs
	return &v
}

func requestParallelToolCalls(opts *config.Options) *bool {
	if opts == nil || opts.ParallelToolCalls == nil {
		return nil
	}
	v := *opts.ParallelToolCalls
	return &v
}

func requestReasoning(opts *config.Options) map[string]any {
	if opts == nil {
		return nil
	}
	reasoning := cloneMap(opts.Reasoning)
	if reasoning == nil {
		reasoning = map[string]any{}
	}
	if opts.Effort != nil {
		reasoning["effort"] = string(*opts.Effort)
	}
	switch t := opts.Thinking.(type) {
	case config.ThinkingConfigDisabled:
		reasoning["enabled"] = false
	case *config.ThinkingConfigDisabled:
		reasoning["enabled"] = false
	case config.ThinkingConfigAdaptive:
		if _, ok := reasoning["effort"]; !ok {
			reasoning["effort"] = "medium"
		}
	case *config.ThinkingConfigAdaptive:
		if _, ok := reasoning["effort"]; !ok {
			reasoning["effort"] = "medium"
		}
	case config.ThinkingConfigEnabled:
		reasoning["enabled"] = true
		if t.BudgetTokens > 0 {
			reasoning["max_tokens"] = t.BudgetTokens
		}
	case *config.ThinkingConfigEnabled:
		reasoning["enabled"] = true
		if t != nil && t.BudgetTokens > 0 {
			reasoning["max_tokens"] = t.BudgetTokens
		}
	}
	if len(reasoning) == 0 {
		return nil
	}
	return reasoning
}

func requestMaxToolCalls(opts *config.Options) *int {
	if opts == nil || opts.MaxToolCalls == nil {
		return nil
	}
	v := *opts.MaxToolCalls
	return &v
}

func requestStreamIncludeUsage(opts *config.Options) *bool {
	if opts == nil || opts.StreamIncludeUsage == nil {
		return nil
	}
	v := *opts.StreamIncludeUsage
	return &v
}

func requestTTL(opts *config.Options) *int {
	if opts == nil || opts.TTL == nil {
		return nil
	}
	v := int(opts.TTL.Seconds())
	if v <= 0 {
		return nil
	}
	return &v
}

func requestDraftModel(opts *config.Options) string {
	if opts == nil {
		return ""
	}
	return strings.TrimSpace(opts.DraftModel)
}

func requestExtra(opts *config.Options) map[string]any {
	if opts == nil {
		return nil
	}
	return cloneMap(opts.Extra)
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func orderCalls(calls map[int]*pendingToolCall) []*pendingToolCall {
	if len(calls) == 0 {
		return nil
	}
	max := -1
	for i := range calls {
		if i > max {
			max = i
		}
	}
	out := make([]*pendingToolCall, 0, len(calls))
	for i := 0; i <= max; i++ {
		if c, ok := calls[i]; ok {
			out = append(out, c)
		}
	}
	return out
}

func effectiveSystemPrompt(opts *config.Options) string {
	if opts == nil {
		return ""
	}
	if opts.SystemPromptPreset != nil {
		if opts.SystemPromptPreset.Append != nil {
			return strings.TrimSpace(*opts.SystemPromptPreset.Append)
		}
		return ""
	}
	return strings.TrimSpace(opts.SystemPrompt)
}

func parseStructuredOutput(text, reasoning string, opts *config.Options) any {
	if opts == nil || len(opts.OutputFormat) == 0 {
		return nil
	}
	if parsed := tryParseJSON(text); parsed != nil {
		return parsed
	}
	// Fallback for LM Studio thinking-mode models (e.g. Qwen3) that route
	// structured-format output into `reasoning_content` instead of `content`.
	return tryParseJSON(reasoning)
}

// tryParseJSON attempts to decode `text` as JSON, tolerating common envelopes
// that non-schema-strict models wrap around the JSON payload:
//
//  1. Raw JSON (fast path).
//  2. Markdown code fence with optional language tag: ```json\n{...}\n```.
//  3. Plain fence without language tag.
//  4. Prose-embedded JSON — locate the first balanced top-level object/array
//     and try to decode just that slice.
//
// Returns the parsed value (map, slice, primitive) or nil if nothing decodes.
func tryParseJSON(text string) any {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	// 1. Fast path.
	var parsed any
	if err := json.Unmarshal([]byte(text), &parsed); err == nil {
		return parsed
	}
	// 2/3. Strip markdown fences if present.
	if stripped := stripCodeFence(text); stripped != text {
		if err := json.Unmarshal([]byte(stripped), &parsed); err == nil {
			return parsed
		}
	}
	// 4. Locate first balanced JSON object or array inside prose.
	if slice := findBalancedJSON(text); slice != "" {
		if err := json.Unmarshal([]byte(slice), &parsed); err == nil {
			return parsed
		}
	}
	return nil
}

// stripCodeFence removes a surrounding ```lang ... ``` fence from text. The
// opening fence may carry an optional language tag (`json`, `JSON`, etc.).
// Returns text unchanged if no fence is present.
func stripCodeFence(text string) string {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "```") {
		return text
	}
	end := strings.LastIndex(trimmed, "```")
	if end <= 3 {
		return text
	}
	inner := trimmed[3:end]
	// Drop optional language tag on the first line.
	if nl := strings.IndexByte(inner, '\n'); nl >= 0 {
		first := strings.TrimSpace(inner[:nl])
		// Heuristic: treat short alpha-only first line as a language tag.
		if first != "" && isLangTag(first) {
			inner = inner[nl+1:]
		}
	}
	return strings.TrimSpace(inner)
}

func isLangTag(s string) bool {
	if len(s) == 0 || len(s) > 16 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// findBalancedJSON scans text for the first JSON object or array that parses
// to a balanced brace/bracket pair. Handles strings (including escapes) so
// that `{` inside a string literal doesn't confuse depth tracking. Returns
// the sliced substring or "" if no balanced candidate is found.
func findBalancedJSON(text string) string {
	start := -1
	var openCh, closeCh byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if c == '{' || c == '[' {
			start = i
			openCh = c
			if c == '{' {
				closeCh = '}'
			} else {
				closeCh = ']'
			}
			break
		}
	}
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	escape := false
	for i := start; i < len(text); i++ {
		c := text[i]
		if inStr {
			switch {
			case escape:
				escape = false
			case c == '\\':
				escape = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case openCh:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

func historyUserContent(content message.UserMessageContent) any {
	if content.IsString() {
		return content.String()
	}
	return content.Blocks()
}

func normalizeAssistantContent(current, incoming string) string {
	if incoming == "" {
		return ""
	}
	if current == "" {
		return incoming
	}
	if strings.HasPrefix(incoming, current) {
		return incoming[len(current):]
	}
	return incoming
}

func assistantContentBlocks(text, reasoning string, images []message.ImageBlock) []message.ContentBlock {
	blocks := make([]message.ContentBlock, 0, 2+len(images))
	if strings.TrimSpace(reasoning) != "" {
		blocks = append(blocks, &message.ThinkingBlock{
			Type:     message.BlockTypeThinking,
			Thinking: reasoning,
		})
	}
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, &message.TextBlock{
			Type: message.BlockTypeText,
			Text: text,
		})
	}
	for _, image := range images {
		img := image
		blocks = append(blocks, &img)
	}
	return blocks
}

func encodeHistoryImages(images []message.ImageBlock) []map[string]any {
	if len(images) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(images))
	for _, image := range images {
		out = append(out, map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": image.URL,
			},
			"media_type": image.MediaType,
		})
	}
	return out
}

func appendNewAssistantImages(existing []message.ImageBlock, incoming []imageDelta) []message.ImageBlock {
	if len(incoming) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(existing))
	for _, image := range existing {
		seen[image.URL] = struct{}{}
	}
	out := make([]message.ImageBlock, 0, len(incoming))
	for _, image := range incoming {
		if image.URL == "" {
			continue
		}
		if _, exists := seen[image.URL]; exists {
			continue
		}
		seen[image.URL] = struct{}{}
		out = append(out, message.ImageBlock{
			Type:      message.BlockTypeImage,
			URL:       image.URL,
			MediaType: image.MediaType,
		})
	}
	return out
}

func (r *QueryRunner) runStream(
	ctx context.Context,
	sessionID string,
	req *config.ChatRequest,
	out chan<- message.Message,
	errs chan<- error,
) (string, string, []message.ImageBlock, map[int]*pendingToolCall, string, *message.Usage, *float64, map[string]any, error, bool) {
	stream, streamErrs := r.transport.CreateStream(ctx, req)

	streamStart := time.Now()
	ttftRecorded := false
	var assistantText strings.Builder
	// assistantEmittedText tracks what has already been streamed as partial
	// AssistantMessage deltas (subset of assistantText). Used to suppress
	// whitespace-only deltas that arrive before any real content is
	// emitted — e.g. the `\n\n` separator LM Studio sends between
	// reasoning_content and content on Qwen3 thinking models.
	var assistantEmittedText strings.Builder
	var assistantReasoning strings.Builder
	assistantImages := []message.ImageBlock{}
	calls := map[int]*pendingToolCall{}
	finishReason := ""
	var usage *message.Usage
	var totalCost *float64
	var terminalEvent map[string]any
	emitted := false

	processEvent := func(ev map[string]any) (bool, error) {
		se := &message.StreamEvent{UUID: "", SessionID: sessionID, Event: ev}
		attachRawAuditEnvelope(se, "stream_event", "", ev)
		select {
		case out <- se:
			emitted = true
		case <-ctx.Done():
			return false, ctx.Err()
		}
		usage, totalCost = parseUsageAndCost(ev, usage, totalCost)

		chunks, err := parseChunk(ev)
		if err != nil {
			select {
			case errs <- err:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			return true, nil
		}
		for _, ch := range chunks {
			if !ttftRecorded && (ch.Content != "" || len(ch.Images) > 0 || len(ch.ToolDeltas) > 0) {
				ttftRecorded = true
				// When no MetricsRecorder is configured, record TTFT
				// directly via the Observer. Otherwise, the recorder
				// handles TTFT from AssistantMessage observation.
				if r.opts.MetricsRecorder == nil {
					r.obs.RecordTTFT(ctx, time.Since(streamStart).Seconds(), req.Model)
				}
			}
			if ch.Reasoning != "" {
				assistantReasoning.WriteString(ch.Reasoning)
				if r.opts.IncludePartialMessages {
					am := &message.AssistantMessage{
						Type:  "assistant",
						Model: req.Model,
						Content: []message.ContentBlock{
							&message.ThinkingBlock{Type: message.BlockTypeThinking, Thinking: ch.Reasoning},
						},
					}
					attachAuditEnvelope(am, "assistant", "partial_reasoning", am)
					select {
					case out <- am:
						emitted = true
					case <-ctx.Done():
						return false, ctx.Err()
					}
				}
			}
			if ch.Content != "" {
				content := normalizeAssistantContent(assistantText.String(), ch.Content)
				if content != "" {
					assistantText.WriteString(content)
				}
				// Suppress partial emission of the whitespace-only separator
				// LM Studio inserts between reasoning_content and the real
				// assistant text (observed on Qwen3 thinking models). Keep
				// the whitespace in the accumulated `assistantText` so the
				// final ResultMessage preserves formatting; just don't
				// stream a bogus `\n\n`-only AssistantMessage. Once real
				// text has been streamed, emit every delta verbatim —
				// mid-reply whitespace carries meaning for consumers.
				emitPartial := r.opts.IncludePartialMessages && content != ""
				if emitPartial && strings.TrimSpace(assistantEmittedText.String()) == "" && strings.TrimSpace(content) == "" {
					emitPartial = false
				}
				if emitPartial {
					assistantEmittedText.WriteString(content)
					am := &message.AssistantMessage{
						Type:  "assistant",
						Model: req.Model,
						Content: []message.ContentBlock{
							&message.TextBlock{Type: message.BlockTypeText, Text: content},
						},
					}
					attachAuditEnvelope(am, "assistant", "partial_text", am)
					r.observeMessage(ctx, am)

					select {
					case out <- am:
						emitted = true
					case <-ctx.Done():
						return false, ctx.Err()
					}
				}
			}
			if len(ch.Images) > 0 {
				newImages := appendNewAssistantImages(assistantImages, ch.Images)
				if len(newImages) > 0 {
					assistantImages = append(assistantImages, newImages...)
					if r.opts.IncludePartialMessages {
						blocks := make([]message.ContentBlock, 0, len(newImages))
						for _, image := range newImages {
							img := image
							blocks = append(blocks, &img)
						}
						am := &message.AssistantMessage{
							Type:    "assistant",
							Model:   req.Model,
							Content: blocks,
						}
						attachAuditEnvelope(am, "assistant", "partial_image", am)
						select {
						case out <- am:
							emitted = true
						case <-ctx.Done():
							return false, ctx.Err()
						}
					}
				}
			}
			for _, td := range ch.ToolDeltas {
				pc := calls[td.Index]
				if pc == nil {
					pc = &pendingToolCall{}
					calls[td.Index] = pc
				}
				if td.ID != "" {
					pc.ID = td.ID
				}
				if td.Name != "" {
					pc.Name = td.Name
				}
				if td.Args != "" {
					pc.Args.WriteString(td.Args)
				}
			}
			if ch.Finish != "" {
				finishReason = ch.Finish
				terminalEvent = ev
			}
		}
		return true, nil
	}

	streamCh := stream
	errCh := streamErrs
	for streamCh != nil || errCh != nil {
		// Prefer already-available stream chunks before handling stream errors
		// so fallback decisions correctly account for emitted output.
		if streamCh != nil {
			select {
			case ev, ok := <-streamCh:
				if !ok {
					streamCh = nil
					continue
				}
				handled, err := processEvent(ev)
				if err != nil {
					return "", "", nil, nil, "", usage, totalCost, terminalEvent, err, emitted
				}
				if handled {
					continue
				}
			default:
			}
		}

		select {
		case ev, ok := <-streamCh:
			if !ok {
				streamCh = nil
				continue
			}
			if _, err := processEvent(ev); err != nil {
				return "", "", nil, nil, "", usage, totalCost, terminalEvent, err, emitted
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				return "", "", nil, nil, "", usage, totalCost, terminalEvent, err, emitted
			}
		case <-ctx.Done():
			return "", "", nil, nil, "", usage, totalCost, terminalEvent, ctx.Err(), emitted
		}
	}

	// Inline tool-call fallback: some LM Studio thinking models (e.g. Qwen3) emit
	// tool calls as <tool_call>...</tool_call> text inside reasoning_content
	// rather than as structured tool_calls[] deltas. If we saw no structured
	// tool calls, scan the reasoning text and synthesise them so the tool
	// loop still runs.
	reasoningText := strings.TrimSpace(assistantReasoning.String())
	if len(calls) == 0 && strings.Contains(reasoningText, "<tool_call>") {
		if synthetic := extractInlineToolCalls(reasoningText); len(synthetic) > 0 {
			for _, td := range synthetic {
				name := td.Name
				if resolved, ok := r.registry.ResolveName(name); ok {
					name = resolved
				}
				pc := &pendingToolCall{ID: td.ID, Name: name}
				pc.Args.WriteString(td.Args)
				calls[td.Index] = pc
			}
			if finishReason == "stop" || finishReason == "" {
				finishReason = "tool_calls"
			}
		}
	}

	return strings.TrimSpace(assistantText.String()), reasoningText, assistantImages, calls, finishReason, usage, totalCost, terminalEvent, nil, emitted
}

type permissionDecision struct {
	deny               bool
	message            string
	interrupt          bool
	updatedInput       map[string]any
	suggestions        []*permission.Update
	updatedPermissions []*permission.Update
}

func permissionDecisionFromHookOutputs(outputs []hook.JSONOutput, fallbackInput map[string]any) permissionDecision {
	out := permissionDecision{updatedInput: fallbackInput}
	for _, o := range outputs {
		syncOut, ok := o.(*hook.SyncJSONOutput)
		if !ok || syncOut == nil {
			continue
		}
		spec, ok := syncOut.HookSpecificOutput.(*hook.PermissionRequestSpecificOutput)
		if !ok || spec == nil || spec.Decision == nil {
			continue
		}
		d := spec.Decision
		if behavior, ok := d["behavior"].(string); ok && behavior == "deny" {
			out.deny = true
		}
		if msg, ok := d["message"].(string); ok && msg != "" {
			out.message = msg
		}
		if intr, ok := d["interrupt"].(bool); ok {
			out.interrupt = intr
		}
		if in, ok := d["updatedInput"].(map[string]any); ok {
			out.updatedInput = in
		}
		if ups, ok := d["updatedPermissions"]; ok {
			out.updatedPermissions = parsePermissionUpdates(ups)
		}
		if sgs, ok := d["suggestions"]; ok {
			out.suggestions = parsePermissionUpdates(sgs)
		}
	}
	return out
}

func parsePermissionUpdates(v any) []*permission.Update {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	updates := make([]*permission.Update, 0, len(raw))
	for _, r := range raw {
		m, ok := r.(map[string]any)
		if !ok {
			continue
		}
		up := &permission.Update{}
		if updateType, ok := m["type"].(string); ok {
			up.Type = permission.UpdateType(updateType)
		}
		if mode, ok := m["mode"].(string); ok {
			mv := permission.Mode(mode)
			up.Mode = &mv
		}
		if behavior, ok := m["behavior"].(string); ok {
			bv := permission.Behavior(behavior)
			up.Behavior = &bv
		}
		if dest, ok := m["destination"].(string); ok {
			dv := permission.UpdateDestination(dest)
			up.Destination = &dv
		}
		if dirs, ok := m["directories"].([]any); ok {
			up.Directories = make([]string, 0, len(dirs))
			for _, d := range dirs {
				if ds, ok := d.(string); ok {
					up.Directories = append(up.Directories, ds)
				}
			}
		}
		if rules, ok := m["rules"].([]any); ok {
			up.Rules = make([]*permission.RuleValue, 0, len(rules))
			for _, rr := range rules {
				rm, ok := rr.(map[string]any)
				if !ok {
					continue
				}
				rule := &permission.RuleValue{}
				if tn, ok := rm["toolName"].(string); ok {
					rule.ToolName = tn
				}
				if rc, ok := rm["ruleContent"].(string); ok {
					rcv := rc
					rule.RuleContent = &rcv
				}
				up.Rules = append(up.Rules, rule)
			}
		}
		updates = append(updates, up)
	}
	return updates
}

func parseUsageAndCost(
	raw map[string]any,
	prevUsage *message.Usage,
	prevCost *float64,
) (*message.Usage, *float64) {
	usage := prevUsage
	cost := prevCost

	parseUsageMap := func(m map[string]any) {
		in := numberFromAny(m["input_tokens"])
		out := numberFromAny(m["output_tokens"])
		if in == 0 && out == 0 {
			in = numberFromAny(m["prompt_tokens"])
			out = numberFromAny(m["completion_tokens"])
		}

		cachedIn := numberFromAny(m["cached_input_tokens"])
		reasoningOut := numberFromAny(m["reasoning_output_tokens"])

		// OpenAI-format nested details.
		if ptd, ok := m["prompt_tokens_details"].(map[string]any); ok {
			if v := numberFromAny(ptd["cached_tokens"]); v != 0 && cachedIn == 0 {
				cachedIn = v
			}
		}
		if ctd, ok := m["completion_tokens_details"].(map[string]any); ok {
			if v := numberFromAny(ctd["reasoning_tokens"]); v != 0 && reasoningOut == 0 {
				reasoningOut = v
			}
		}

		if in != 0 || out != 0 {
			usage = &message.Usage{
				InputTokens:           in,
				OutputTokens:          out,
				CachedInputTokens:     cachedIn,
				ReasoningOutputTokens: reasoningOut,
			}
		}
		if v, ok := floatFromAny(m["total_cost_usd"]); ok {
			cost = &v
		}
	}

	if u, ok := raw["usage"].(map[string]any); ok {
		parseUsageMap(u)
	}
	if v, ok := floatFromAny(raw["total_cost_usd"]); ok {
		cost = &v
	}
	if response, ok := raw["response"].(map[string]any); ok {
		if u, ok := response["usage"].(map[string]any); ok {
			parseUsageMap(u)
		}
		if v, ok := floatFromAny(response["total_cost_usd"]); ok {
			cost = &v
		}
	}

	return usage, cost
}

func numberFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func floatFromAny(v any) (float64, bool) {
	switch n := v.(type) {
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
