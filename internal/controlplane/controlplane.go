package controlplane

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/tools"
)

// Control identifies a logical control-plane operation.
type Control string

const (
	ControlInitialize        Control = "initialize"
	ControlInterrupt         Control = "interrupt"
	ControlSetModel          Control = "set_model"
	ControlSetPermissionMode Control = "set_permission_mode"
	ControlRewindFiles       Control = "rewind_files"
	ControlMCPStatus         Control = "mcp_status"
)

// ErrUnsupportedControl is a sentinel error for unsupported control operations.
var ErrUnsupportedControl = errors.New("unsupported control operation")

// UnsupportedControlError is returned when a control op has no backend equivalent.
type UnsupportedControlError struct {
	Control Control
	Backend string
	Reason  string
}

func (e *UnsupportedControlError) Error() string {
	backend := e.Backend
	if backend == "" {
		backend = "unknown"
	}
	if e.Reason == "" {
		return fmt.Sprintf("control %q unsupported by backend %q", e.Control, backend)
	}
	return fmt.Sprintf("control %q unsupported by backend %q: %s", e.Control, backend, e.Reason)
}

func (e *UnsupportedControlError) Unwrap() error { return ErrUnsupportedControl }

// Runner exposes runner methods needed for control operations.
type Runner interface {
	SetPermissionMode(mode string)
}

// Host exposes client state needed by the local control plane.
type Host interface {
	Connected() bool
	CurrentCancel() context.CancelFunc
	Options() *config.Options
	Runner() Runner
	ActiveSessionID() string
	Rewind(sessionID, userMessageID string) error
	SetMCPStatus(map[string]mcp.ServerStatus)
	MCPStatus() map[string]mcp.ServerStatus
}

// Controller handles mutable runtime control operations.
type Controller interface {
	Initialize(ctx context.Context) error
	Interrupt(ctx context.Context) error
	SetPermissionMode(ctx context.Context, mode string) error
	SetModel(ctx context.Context, model *string) error
	GetMCPStatus(ctx context.Context) (*mcp.Status, error)
	RewindFiles(ctx context.Context, userMessageID string) error
}

// NewLocalController returns an in-process controller for runtime state.
func NewLocalController(host Host) Controller {
	return &localController{
		host:    host,
		backend: "lmstudio",
		refreshFn: func(opts *config.Options) map[string]mcp.ServerStatus {
			return tools.NewRegistry(opts).ServerStatuses()
		},
	}
}

type localController struct {
	host      Host
	backend   string
	refreshFn func(*config.Options) map[string]mcp.ServerStatus
	mu        sync.Mutex
	refresh   bool
}

func (c *localController) Initialize(context.Context) error {
	return &UnsupportedControlError{
		Control: ControlInitialize,
		Backend: c.backend,
		Reason:  "protocol initialize lifecycle is not available in this SDK",
	}
}

func (c *localController) Interrupt(ctx context.Context) error {
	if c.host == nil || !c.host.Connected() {
		return sdkerrors.ErrClientNotConnected
	}
	cancel := c.host.CurrentCancel()
	if cancel == nil {
		return sdkerrors.ErrClientNotConnected
	}
	cancel()
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (c *localController) SetPermissionMode(_ context.Context, mode string) error {
	if c.host == nil || !c.host.Connected() {
		return sdkerrors.ErrClientNotConnected
	}
	opts := c.host.Options()
	if opts == nil {
		return sdkerrors.ErrClientNotConnected
	}
	opts.PermissionMode = mode
	if runner := c.host.Runner(); runner != nil {
		runner.SetPermissionMode(mode)
	}
	return nil
}

func (c *localController) SetModel(_ context.Context, model *string) error {
	if c.host == nil || !c.host.Connected() {
		return sdkerrors.ErrClientNotConnected
	}
	opts := c.host.Options()
	if opts == nil {
		return sdkerrors.ErrClientNotConnected
	}
	if model == nil {
		opts.Model = ""
		opts.ApplyDefaults()
		return nil
	}
	opts.Model = *model
	return nil
}

func (c *localController) GetMCPStatus(_ context.Context) (*mcp.Status, error) {
	if c.host == nil || !c.host.Connected() {
		return nil, sdkerrors.ErrClientNotConnected
	}
	opts := c.host.Options()
	if opts != nil && len(opts.MCPServers) > 0 {
		current := c.host.MCPStatus()
		if len(current) == 0 {
			initial := make(map[string]mcp.ServerStatus, len(opts.MCPServers))
			for name := range opts.MCPServers {
				initial[name] = mcp.ServerStatus{Name: name, Status: "configured"}
			}
			c.host.SetMCPStatus(initial)
		} else if c.anyInState(current, "connecting", "configured") {
			c.startMCPRefresh(opts)
		}
	}

	m := c.host.MCPStatus()
	out := &mcp.Status{MCPServers: make([]mcp.ServerStatus, 0, len(m))}
	for _, s := range m {
		out.MCPServers = append(out.MCPServers, s)
	}
	return out, nil
}

func (c *localController) RewindFiles(_ context.Context, userMessageID string) error {
	if c.host == nil || !c.host.Connected() {
		return sdkerrors.ErrClientNotConnected
	}
	opts := c.host.Options()
	if opts == nil || !opts.EnableFileCheckpointing {
		return &UnsupportedControlError{
			Control: c.controlID(ControlRewindFiles),
			Backend: c.backend,
			Reason:  "file checkpointing is disabled",
		}
	}
	sessionID := c.host.ActiveSessionID()
	if sessionID == "" {
		sessionID = "default"
	}
	if err := c.host.Rewind(sessionID, userMessageID); err != nil {
		return err
	}
	return nil
}

func (c *localController) controlID(control Control) Control {
	if control == "" {
		return Control("unknown")
	}
	return control
}

func (c *localController) startMCPRefresh(opts *config.Options) {
	c.mu.Lock()
	if c.refresh {
		c.mu.Unlock()
		return
	}
	c.refresh = true
	c.mu.Unlock()

	current := c.host.MCPStatus()
	if len(current) > 0 {
		next := make(map[string]mcp.ServerStatus, len(current))
		for name, st := range current {
			st.Name = name
			st.Status = "connecting"
			st.Reason = ""
			next[name] = st
		}
		c.host.SetMCPStatus(next)
	}

	go func() {
		latest := c.refreshFn(opts)
		c.host.SetMCPStatus(latest)
		c.mu.Lock()
		c.refresh = false
		c.mu.Unlock()
	}()
}

func (c *localController) anyInState(statuses map[string]mcp.ServerStatus, states ...string) bool {
	if len(statuses) == 0 || len(states) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(states))
	for _, s := range states {
		set[s] = struct{}{}
	}
	for _, st := range statuses {
		if _, ok := set[st.Status]; ok {
			return true
		}
	}
	return false
}
