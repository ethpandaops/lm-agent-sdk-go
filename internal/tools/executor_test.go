package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/permission"
)

type testServer struct{}

func (s *testServer) Name() string    { return "test" }
func (s *testServer) Version() string { return "1.0.0" }
func (s *testServer) ListTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "echo",
			"description": "echo",
			"inputSchema": map[string]any{"type": "object"},
		},
	}
}
func (s *testServer) CallTool(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": "ok"},
		},
	}, nil
}

func TestExecutorAppliesUpdatedPermissions(t *testing.T) {
	deny := permission.BehaviorDeny
	opts := &config.Options{
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &testServer{}},
		},
		CanUseTool: func(_ context.Context, _ string, input map[string]any, _ *permission.Context) (permission.Result, error) {
			return &permission.ResultAllow{
				Behavior:     "allow",
				UpdatedInput: input,
				UpdatedPermissions: []*permission.Update{
					{
						Type:     permission.UpdateTypeAddRules,
						Behavior: &deny,
						Rules: []*permission.RuleValue{
							{ToolName: "mcp__srv__echo"},
						},
					},
				},
			}, nil
		},
	}
	reg := NewRegistry(opts)
	exec := NewExecutor(opts, reg)

	out, err := exec.Execute(context.Background(), "mcp__srv__echo", map[string]any{"v": 1})
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}

	_, err = exec.Execute(context.Background(), "mcp__srv__echo", map[string]any{"v": 2})
	var denyErr *sdkerrors.ToolPermissionDeniedError
	if err == nil || !errors.As(err, &denyErr) {
		t.Fatalf("expected tool permission denied error, got %v", err)
	}
}

func TestExecutorDenyInterrupt(t *testing.T) {
	opts := &config.Options{
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &testServer{}},
		},
		CanUseTool: func(_ context.Context, _ string, _ map[string]any, _ *permission.Context) (permission.Result, error) {
			return &permission.ResultDeny{
				Behavior:  "deny",
				Message:   "blocked",
				Interrupt: true,
			}, nil
		},
	}
	reg := NewRegistry(opts)
	exec := NewExecutor(opts, reg)

	_, err := exec.Execute(context.Background(), "mcp__srv__echo", map[string]any{})
	var denyErr *sdkerrors.ToolPermissionDeniedError
	if err == nil || !errors.As(err, &denyErr) {
		t.Fatalf("expected denied error, got %v", err)
	}
	if !denyErr.Interrupt {
		t.Fatalf("expected interrupt=true")
	}
}

func TestExecutorPassesSuggestionsToCallback(t *testing.T) {
	seen := 0
	opts := &config.Options{
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &testServer{}},
		},
		CanUseTool: func(_ context.Context, _ string, _ map[string]any, permCtx *permission.Context) (permission.Result, error) {
			seen = len(permCtx.Suggestions)
			return &permission.ResultAllow{Behavior: "allow"}, nil
		},
	}
	reg := NewRegistry(opts)
	exec := NewExecutor(opts, reg)
	behavior := permission.BehaviorAllow
	suggestions := []*permission.Update{
		{
			Type:     permission.UpdateTypeAddRules,
			Behavior: &behavior,
			Rules: []*permission.RuleValue{
				{ToolName: "mcp__srv__echo"},
			},
		},
	}

	if _, err := exec.ExecuteWithSuggestions(context.Background(), "mcp__srv__echo", map[string]any{}, suggestions); err != nil {
		t.Fatalf("execute with suggestions: %v", err)
	}
	if seen != 1 {
		t.Fatalf("expected 1 suggestion passed to callback, got %d", seen)
	}
}

func TestExecutorLoadsPersistedProjectRules(t *testing.T) {
	tmp := t.TempDir()
	deny := permission.BehaviorDeny
	store := permission.NewStore(tmp)
	if err := store.ApplyPersistentUpdate(&permission.Update{
		Type:        permission.UpdateTypeAddRules,
		Behavior:    &deny,
		Destination: ptrDest(permission.UpdateDestProjectSettings),
		Rules: []*permission.RuleValue{
			{ToolName: "mcp__srv__echo"},
		},
	}); err != nil {
		t.Fatalf("persist update: %v", err)
	}

	opts := &config.Options{
		Cwd: tmp,
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &testServer{}},
		},
	}
	reg := NewRegistry(opts)
	exec := NewExecutor(opts, reg)

	_, err := exec.Execute(context.Background(), "mcp__srv__echo", map[string]any{})
	var denyErr *sdkerrors.ToolPermissionDeniedError
	if err == nil || !errors.As(err, &denyErr) {
		t.Fatalf("expected denied error from persisted rule, got %v", err)
	}
}

func ptrDest(d permission.UpdateDestination) *permission.UpdateDestination { return &d }

func TestExecutorHonorsInitialBypassPermissionMode(t *testing.T) {
	deny := permission.BehaviorDeny
	opts := &config.Options{
		PermissionMode: string(permission.ModeBypassPermissions),
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &testServer{}},
		},
	}
	reg := NewRegistry(opts)
	exec := NewExecutor(opts, reg)
	exec.ApplyPermissionUpdates([]*permission.Update{
		{
			Type:     permission.UpdateTypeAddRules,
			Behavior: &deny,
			Rules: []*permission.RuleValue{
				{ToolName: "mcp__srv__echo"},
			},
		},
	})

	out, err := exec.Execute(context.Background(), "mcp__srv__echo", map[string]any{})
	if err != nil {
		t.Fatalf("expected bypass mode to ignore deny rules, got %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecutorSetModeUpdatesEvaluation(t *testing.T) {
	deny := permission.BehaviorDeny
	opts := &config.Options{
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &testServer{}},
		},
	}
	reg := NewRegistry(opts)
	exec := NewExecutor(opts, reg)
	exec.ApplyPermissionUpdates([]*permission.Update{
		{
			Type:     permission.UpdateTypeAddRules,
			Behavior: &deny,
			Rules: []*permission.RuleValue{
				{ToolName: "mcp__srv__echo"},
			},
		},
	})

	_, err := exec.Execute(context.Background(), "mcp__srv__echo", map[string]any{})
	var denyErr *sdkerrors.ToolPermissionDeniedError
	if err == nil || !errors.As(err, &denyErr) {
		t.Fatalf("expected denied error before bypass mode, got %v", err)
	}

	exec.SetMode(permission.ModeBypassPermissions)
	out, err := exec.Execute(context.Background(), "mcp__srv__echo", map[string]any{})
	if err != nil {
		t.Fatalf("expected bypass mode to allow tool, got %v", err)
	}
	if out != "ok" {
		t.Fatalf("unexpected output after bypass mode: %q", out)
	}
}
