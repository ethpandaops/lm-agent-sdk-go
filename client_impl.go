package lmsdk

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"os"
	"strings"
	"sync"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/controlplane"
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/lmstudio"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/message"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/runtime"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/session"
)

type clientImpl struct {
	mu        sync.Mutex
	options   *config.Options
	sessions  *session.Manager
	engine    *runtime.Engine
	mcpStatus map[string]mcp.ServerStatus
	connected bool
	closed    bool
	control   controlplane.Controller

	currentMsgs   <-chan message.Message
	currentErrs   <-chan error
	currentCancel context.CancelFunc

	activeSession string
}

var _ Client = (*clientImpl)(nil)

func newClientImpl() Client {
	return &clientImpl{
		sessions:      session.NewManager(),
		activeSession: "default",
		mcpStatus:     map[string]mcp.ServerStatus{},
	}
}

func (c *clientImpl) Start(ctx context.Context, opts ...Option) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return sdkerrors.ErrClientClosed
	}
	if c.connected {
		return sdkerrors.ErrClientAlreadyConnected
	}

	cfg := applyAgentOptionsToConfig(opts)
	if cfg == nil {
		cfg = &config.Options{}
	}
	if err := applyMCPConfigToOptions(cfg); err != nil {
		return err
	}
	cfg.ApplyDefaults()
	if err := validateAndConfigureCompatibilityOptions(cfg); err != nil {
		return err
	}
	if err := runtime.ValidateHookSupport(cfg.Hooks); err != nil {
		return err
	}

	c.options = cfg
	if err := c.configureSessionPersistence(cfg); err != nil {
		return err
	}
	c.engine = runtime.NewEngine(cfg, c.sessions)
	c.activeSession = c.resolveInitialSession(cfg)
	c.initMCPStatus(c.engine.Runner(), cfg)
	c.control = controlplane.NewLocalController(c)
	if err := c.control.Initialize(ctx); err != nil && !errors.Is(err, controlplane.ErrUnsupportedControl) {
		return err
	}
	c.connected = true
	return nil
}

func (c *clientImpl) StartWithContent(ctx context.Context, content UserMessageContent, opts ...Option) error {
	if err := c.Start(ctx, opts...); err != nil {
		return err
	}
	return c.Query(ctx, content)
}

func (c *clientImpl) StartWithStream(
	ctx context.Context,
	messages iter.Seq[StreamingMessage],
	opts ...Option,
) error {
	if err := c.Start(ctx, opts...); err != nil {
		return err
	}
	return c.runMessageIterator(ctx, c.activeSession, messages)
}

func (c *clientImpl) Query(ctx context.Context, content UserMessageContent, sessionID ...string) error {
	if !c.isConnected() {
		return sdkerrors.ErrClientNotConnected
	}
	sid := c.activeSession
	if len(sessionID) > 0 && sessionID[0] != "" {
		sid = sessionID[0]
	}
	return c.runContent(ctx, sid, content)
}

func (c *clientImpl) runContent(ctx context.Context, sid string, content UserMessageContent) error {
	qctx, cancel := context.WithCancel(ctx)
	msgs, errs := c.engine.Runner().RunPrompt(qctx, sid, content)
	c.setCurrent(msgs, errs, cancel, sid)
	return nil
}

func (c *clientImpl) runMessageIterator(
	ctx context.Context,
	sid string,
	inputs iter.Seq[StreamingMessage],
) error {
	qctx, cancel := context.WithCancel(ctx)
	msgs, errs := c.engine.Runner().RunMessageIterator(qctx, sid, inputs)
	c.setCurrent(msgs, errs, cancel, sid)
	return nil
}

func (c *clientImpl) setCurrent(
	msgs <-chan message.Message,
	errs <-chan error,
	cancel context.CancelFunc,
	sessionID string,
) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.currentCancel != nil {
		c.currentCancel()
	}
	c.currentMsgs = msgs
	c.currentErrs = errs
	c.currentCancel = cancel
	c.activeSession = sessionID
}

func (c *clientImpl) ReceiveMessages(ctx context.Context) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		msgs, errs, err := c.getCurrentStreams()
		if err != nil {
			yield(nil, err)
			return
		}

		msgsClosed := false
		errsClosed := false
		for !msgsClosed || !errsClosed {
			select {
			case msg, ok := <-msgs:
				if !ok {
					msgsClosed = true
					continue
				}
				if !yield(msg, nil) {
					return
				}
			case err, ok := <-errs:
				if !ok {
					errsClosed = true
					continue
				}
				if err != nil {
					if !yield(nil, err) {
						return
					}
				}
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			}
		}
	}
}

func (c *clientImpl) ReceiveResponse(ctx context.Context) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		msgs, errs, err := c.getCurrentStreams()
		if err != nil {
			yield(nil, err)
			return
		}

		msgsClosed := false
		errsClosed := false
		resultSeen := false

		for !msgsClosed || !errsClosed {
			select {
			case msg, ok := <-msgs:
				if !ok {
					msgsClosed = true
					continue
				}
				if resultSeen {
					continue
				}
				if !yield(msg, nil) {
					return
				}
				if _, ok := msg.(*message.ResultMessage); ok {
					resultSeen = true
				}
			case err, ok := <-errs:
				if !ok {
					errsClosed = true
					continue
				}
				if err == nil || resultSeen {
					continue
				}
				if !yield(nil, err) {
					return
				}
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			}
		}
	}
}

func (c *clientImpl) Interrupt(ctx context.Context) error {
	controller := c.getControlPlane()
	if controller == nil {
		return sdkerrors.ErrClientNotConnected
	}
	return controller.Interrupt(ctx)
}

func (c *clientImpl) SetPermissionMode(ctx context.Context, mode string) error {
	controller := c.getControlPlane()
	if controller == nil {
		return sdkerrors.ErrClientNotConnected
	}
	return controller.SetPermissionMode(ctx, mode)
}

func (c *clientImpl) SetModel(ctx context.Context, model *string) error {
	controller := c.getControlPlane()
	if controller == nil {
		return sdkerrors.ErrClientNotConnected
	}
	return controller.SetModel(ctx, model)
}

func (c *clientImpl) ListModels(ctx context.Context) ([]ModelInfo, error) {
	resp, err := c.ListModelsResponse(ctx)
	if err != nil {
		return nil, err
	}
	return resp.Models, nil
}

func (c *clientImpl) ListModelsResponse(ctx context.Context) (*ModelListResponse, error) {
	if !c.isConnected() {
		return nil, sdkerrors.ErrClientNotConnected
	}
	c.mu.Lock()
	cfg := c.options
	c.mu.Unlock()
	return lmstudio.ListModelsResponse(ctx, cfg)
}

func (c *clientImpl) GetServerInfo() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected || c.options == nil {
		return nil
	}
	return map[string]any{
		"provider":   "lmstudio",
		"base_url":   c.options.BaseURL,
		"model":      c.options.Model,
		"session_id": c.activeSession,
	}
}

func (c *clientImpl) GetMCPStatus(ctx context.Context) (*MCPStatus, error) {
	controller := c.getControlPlane()
	if controller == nil {
		return nil, sdkerrors.ErrClientNotConnected
	}
	return controller.GetMCPStatus(ctx)
}

func (c *clientImpl) ReconnectMCPServer(context.Context, string) error {
	return unsupportedControl(controlplane.Control("reconnect_mcp_server"), "vLLM MCP reconnection is not separately addressable")
}

func (c *clientImpl) ToggleMCPServer(context.Context, string, bool) error {
	return unsupportedControl(controlplane.Control("toggle_mcp_server"), "vLLM MCP server enablement is fixed for the active runtime")
}

func (c *clientImpl) StopTask(context.Context, string) error {
	return unsupportedControl(controlplane.Control("stop_task"), "vLLM does not expose remote task handles to stop after dispatch")
}

func (c *clientImpl) RewindFiles(ctx context.Context, userMessageID string) error {
	controller := c.getControlPlane()
	if controller == nil {
		return sdkerrors.ErrClientNotConnected
	}
	return controller.RewindFiles(ctx, userMessageID)
}

func (c *clientImpl) SendToolResult(context.Context, string, string, bool) error {
	return unsupportedControl(controlplane.Control("send_tool_result"), "vLLM tool loops are SDK-owned and do not support out-of-band tool result injection")
}

func (c *clientImpl) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	if c.currentCancel != nil {
		c.currentCancel()
	}
	if c.options != nil && c.options.Transport != nil {
		_ = c.options.Transport.Close()
	}
	c.control = nil
	c.closed = true
	c.connected = false
	return nil
}

func (c *clientImpl) getControlPlane() controlplane.Controller {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected || c.closed {
		return nil
	}
	return c.control
}

func (c *clientImpl) isConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected && !c.closed
}

func (c *clientImpl) getCurrentStreams() (<-chan message.Message, <-chan error, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.connected {
		return nil, nil, sdkerrors.ErrClientNotConnected
	}
	if c.currentMsgs == nil || c.currentErrs == nil {
		return nil, nil, sdkerrors.ErrClientNotConnected
	}
	return c.currentMsgs, c.currentErrs, nil
}

func applyAgentOptionsToConfig(opts []Option) *config.Options {
	options := applyAgentOptions(opts)
	if options == nil {
		return nil
	}

	// Initialize OTel metrics recorder from providers if configured.
	initMetricsRecorder(options)

	return options
}

func validateAndConfigureCompatibilityOptions(cfg *config.Options) error {
	if cfg == nil {
		return nil
	}
	if cfg.CanUseTool != nil {
		if cfg.PermissionPromptToolName != "" {
			return fmt.Errorf("can_use_tool callback cannot be used with permission_prompt_tool_name")
		}
		cfg.PermissionPromptToolName = "stdio"
	}
	if cfg.OnUserInput != nil {
		if cfg.PermissionPromptToolName != "" {
			return fmt.Errorf("on_user_input callback cannot be used with permission_prompt_tool_name")
		}
		cfg.PermissionPromptToolName = "stdio"
		ensureUserInputTool(cfg)
	}
	return nil
}

func applyMCPConfigToOptions(cfg *config.Options) error {
	if cfg == nil || strings.TrimSpace(cfg.MCPConfig) == "" {
		return nil
	}
	servers, err := loadMCPConfig(cfg.MCPConfig)
	if err != nil {
		return err
	}
	cfg.MCPServers = servers
	return nil
}

func (c *clientImpl) resolveInitialSession(cfg *config.Options) string {
	if cfg == nil {
		return "default"
	}

	if cfg.Resume == "" {
		return "default"
	}

	if !cfg.ForkSession {
		c.sessions.GetOrCreate(cfg.Resume)
		return cfg.Resume
	}

	forkID := c.sessions.NewForkID(cfg.Resume)
	if !c.sessions.Clone(cfg.Resume, forkID) {
		c.sessions.GetOrCreate(forkID)
	}
	return forkID
}

func (c *clientImpl) initMCPStatus(runner *runtime.QueryRunner, cfg *config.Options) {
	if runner != nil {
		c.mcpStatus = runner.MCPServerStatuses()
		return
	}
	c.mcpStatus = make(map[string]mcp.ServerStatus, len(cfg.MCPServers))
}

// controlplane.Host implementation

func (c *clientImpl) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected && !c.closed
}

func (c *clientImpl) CurrentCancel() context.CancelFunc {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.currentCancel
}

func (c *clientImpl) Options() *config.Options {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.options
}

func (c *clientImpl) Runner() controlplane.Runner {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.engine == nil {
		return nil
	}
	return c.engine.Runner()
}

func (c *clientImpl) ActiveSessionID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.activeSession
}

func (c *clientImpl) Rewind(sessionID, userMessageID string) error {
	if ok := c.sessions.Rewind(sessionID, userMessageID); !ok {
		return session.ErrNoCheckpoint
	}
	c.mu.Lock()
	cfg := c.options
	c.mu.Unlock()
	if cfg != nil && cfg.EnableFileCheckpointing && cfg.Cwd != "" {
		if err := c.sessions.RewindFiles(sessionID, userMessageID, cfg.Cwd); err != nil {
			return err
		}
	}
	return nil
}

func (c *clientImpl) SetMCPStatus(statuses map[string]mcp.ServerStatus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.mcpStatus = statuses
}

func (c *clientImpl) MCPStatus() map[string]mcp.ServerStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]mcp.ServerStatus, len(c.mcpStatus))
	for k, v := range c.mcpStatus {
		out[k] = v
	}
	return out
}

func (c *clientImpl) configureSessionPersistence(cfg *config.Options) error {
	path := ""
	if cfg != nil {
		path = cfg.SessionStorePath
	}
	if path == "" {
		path = os.Getenv("LM_AGENT_SESSION_STORE_PATH")
	}
	if path == "" {
		return nil
	}
	return c.sessions.EnablePersistence(path)
}

func unsupportedControl(control controlplane.Control, reason string) error {
	return &controlplane.UnsupportedControlError{
		Control: control,
		Backend: "vllm",
		Reason:  reason,
	}
}
