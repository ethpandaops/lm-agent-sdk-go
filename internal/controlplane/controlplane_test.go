package controlplane

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/session"
)

type stubRunner struct {
	permissionMode string
}

func (s *stubRunner) SetPermissionMode(mode string) {
	s.permissionMode = mode
}

type stubHost struct {
	connected   bool
	cancel      context.CancelFunc
	opts        *config.Options
	runner      Runner
	sessionID   string
	rewindErr   error
	mu          sync.Mutex
	mcpStatuses map[string]mcp.ServerStatus
}

func (h *stubHost) Connected() bool                   { return h.connected }
func (h *stubHost) CurrentCancel() context.CancelFunc { return h.cancel }
func (h *stubHost) Options() *config.Options          { return h.opts }
func (h *stubHost) Runner() Runner                    { return h.runner }
func (h *stubHost) ActiveSessionID() string           { return h.sessionID }
func (h *stubHost) Rewind(_ string, _ string) error   { return h.rewindErr }
func (h *stubHost) SetMCPStatus(status map[string]mcp.ServerStatus) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mcpStatuses = status
}
func (h *stubHost) MCPStatus() map[string]mcp.ServerStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]mcp.ServerStatus, len(h.mcpStatuses))
	for k, v := range h.mcpStatuses {
		out[k] = v
	}
	return out
}

func TestLocalControllerSetPermissionModeUpdatesOptionsAndRunner(t *testing.T) {
	r := &stubRunner{}
	host := &stubHost{
		connected: true,
		opts:      &config.Options{},
		runner:    r,
	}
	c := NewLocalController(host)
	if err := c.SetPermissionMode(context.Background(), "acceptEdits"); err != nil {
		t.Fatalf("set permission mode: %v", err)
	}

	if host.opts.PermissionMode != "acceptEdits" {
		t.Fatalf("expected opts permission mode update, got %q", host.opts.PermissionMode)
	}
	if r.permissionMode != "acceptEdits" {
		t.Fatalf("expected runner permission mode update, got %q", r.permissionMode)
	}
}

func TestLocalControllerSetModelNilResetsToDefault(t *testing.T) {
	host := &stubHost{
		connected: true,
		opts: &config.Options{
			Model: "custom-model",
		},
	}
	c := NewLocalController(host)
	if err := c.SetModel(context.Background(), nil); err != nil {
		t.Fatalf("set model: %v", err)
	}
	// Model is cleared; ApplyDefaults resolves from LM_MODEL env var.
	t.Setenv("LM_MODEL", "env-model")
	host.opts.Model = ""
	host.opts.ApplyDefaults()
	if host.opts.Model != "env-model" {
		t.Fatalf("expected model from env, got %q", host.opts.Model)
	}
}

func TestLocalControllerSetModelNilClearsWithoutEnv(t *testing.T) {
	t.Setenv("LM_MODEL", "")
	host := &stubHost{
		connected: true,
		opts: &config.Options{
			Model: "custom-model",
		},
	}
	c := NewLocalController(host)
	if err := c.SetModel(context.Background(), nil); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if host.opts.Model != "" {
		t.Fatalf("expected empty model, got %q", host.opts.Model)
	}
}

func TestLocalControllerRewindFilesDisabledReturnsTypedUnsupported(t *testing.T) {
	host := &stubHost{
		connected: true,
		opts:      &config.Options{},
	}
	c := NewLocalController(host)
	err := c.RewindFiles(context.Background(), "msg-1")

	var unsupported *UnsupportedControlError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedControlError, got %v", err)
	}
	if unsupported.Control != ControlRewindFiles {
		t.Fatalf("unexpected control: %q", unsupported.Control)
	}
	if unsupported.Backend != "lmstudio" {
		t.Fatalf("unexpected backend: %q", unsupported.Backend)
	}
	if !errors.Is(err, ErrUnsupportedControl) {
		t.Fatalf("expected errors.Is(err, ErrUnsupportedControl)")
	}
}

func TestLocalControllerRewindFilesPropagatesNoCheckpoint(t *testing.T) {
	host := &stubHost{
		connected: true,
		opts: &config.Options{
			EnableFileCheckpointing: true,
		},
		sessionID: "default",
		rewindErr: session.ErrNoCheckpoint,
	}
	c := NewLocalController(host)
	err := c.RewindFiles(context.Background(), "msg-1")
	if !errors.Is(err, session.ErrNoCheckpoint) {
		t.Fatalf("expected no checkpoint error, got %v", err)
	}
}

func TestLocalControllerInterruptRequiresConnectedClient(t *testing.T) {
	host := &stubHost{connected: false}
	c := NewLocalController(host)
	err := c.Interrupt(context.Background())
	if !errors.Is(err, sdkerrors.ErrClientNotConnected) {
		t.Fatalf("expected not connected error, got %v", err)
	}
}

func TestLocalControllerInitializeReturnsTypedUnsupported(t *testing.T) {
	host := &stubHost{connected: true, opts: &config.Options{}}
	ctrl := NewLocalController(host).(*localController)
	err := ctrl.Initialize(context.Background())
	var unsupported *UnsupportedControlError
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected UnsupportedControlError, got %v", err)
	}
	if unsupported.Control != ControlInitialize {
		t.Fatalf("unexpected control: %q", unsupported.Control)
	}
	if !errors.Is(err, ErrUnsupportedControl) {
		t.Fatalf("expected ErrUnsupportedControl")
	}
}

func TestLocalControllerGetMCPStatusTransitionsConnectingToReady(t *testing.T) {
	host := &stubHost{
		connected: true,
		opts: &config.Options{
			MCPServers: map[string]mcp.ServerConfig{
				"srv": nil,
			},
		},
		mcpStatuses: map[string]mcp.ServerStatus{},
	}
	ctrl := NewLocalController(host).(*localController)
	ctrl.refreshFn = func(_ *config.Options) map[string]mcp.ServerStatus {
		time.Sleep(10 * time.Millisecond)
		return map[string]mcp.ServerStatus{
			"srv": {Name: "srv", Status: "error", Reason: "missing config"},
		}
	}

	st1, err := ctrl.GetMCPStatus(context.Background())
	if err != nil {
		t.Fatalf("get status 1: %v", err)
	}
	if len(st1.MCPServers) != 1 || st1.MCPServers[0].Status != "configured" {
		t.Fatalf("expected configured state first, got %+v", st1.MCPServers)
	}

	st2, err := ctrl.GetMCPStatus(context.Background())
	if err != nil {
		t.Fatalf("get status 2: %v", err)
	}
	if len(st2.MCPServers) != 1 || st2.MCPServers[0].Status != "connecting" {
		t.Fatalf("expected connecting state second, got %+v", st2.MCPServers)
	}

	time.Sleep(20 * time.Millisecond)

	st3, err := ctrl.GetMCPStatus(context.Background())
	if err != nil {
		t.Fatalf("get status 3: %v", err)
	}
	if len(st3.MCPServers) != 1 || st3.MCPServers[0].Status != "error" {
		t.Fatalf("expected refreshed error state, got %+v", st3.MCPServers)
	}
}
