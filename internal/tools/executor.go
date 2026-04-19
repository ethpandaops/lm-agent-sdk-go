package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/permission"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Executor executes tools from the registry.
type Executor struct {
	opts *config.Options
	reg  *Registry

	mu    sync.Mutex
	rules map[string]permission.Behavior
	mode  *permission.Mode
	store *permission.Store
}

// NewExecutor creates a tool executor.
func NewExecutor(opts *config.Options, reg *Registry) *Executor {
	exec := &Executor{
		opts:  opts,
		reg:   reg,
		rules: make(map[string]permission.Behavior, 16),
	}
	if opts != nil && opts.PermissionMode != "" {
		mode := permission.Mode(opts.PermissionMode)
		exec.mode = &mode
	}

	cwd := ""
	if opts != nil {
		cwd = opts.Cwd
	}
	exec.store = permission.NewStore(cwd)
	if exec.store != nil {
		if pol, err := exec.store.LoadMerged(); err == nil && pol != nil {
			if pol.Mode != nil {
				exec.mode = pol.Mode
			}
			for k, v := range pol.Rules {
				exec.rules[k] = v
			}
		}
	}
	return exec
}

// Execute executes one tool call and returns a text payload.
func (e *Executor) Execute(ctx context.Context, name string, input map[string]any) (string, error) {
	return e.ExecuteWithSuggestions(ctx, name, input, nil)
}

// ExecuteWithSuggestions executes one tool call and provides permission suggestions to callback.
func (e *Executor) ExecuteWithSuggestions(
	ctx context.Context,
	name string,
	input map[string]any,
	suggestions []*permission.Update,
) (string, error) {
	ref, ok := e.reg.Get(name)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", name)
	}

	if deny, reason, interrupt := e.evaluatePolicy(name); deny {
		return "", &sdkerrors.ToolPermissionDeniedError{
			ToolName:  name,
			Message:   reason,
			Interrupt: interrupt,
		}
	}

	if e.opts != nil && e.opts.CanUseTool != nil {
		permCtx := &permission.Context{Suggestions: suggestions}
		res, err := e.opts.CanUseTool(ctx, name, input, permCtx)
		if err != nil {
			return "", err
		}
		if res == nil {
			res = &permission.ResultAllow{Behavior: "allow"}
		}
		if deny, ok := res.(*permission.ResultDeny); ok {
			reason := deny.Message
			if reason == "" {
				reason = "tool denied: " + name
			}
			return "", &sdkerrors.ToolPermissionDeniedError{
				ToolName:  name,
				Message:   reason,
				Interrupt: deny.Interrupt,
			}
		}
		allow, ok := res.(*permission.ResultAllow)
		if !ok {
			return "", fmt.Errorf(
				"tool permission callback must return *permission.ResultAllow or *permission.ResultDeny, got %T",
				res,
			)
		}
		if allow.UpdatedInput != nil {
			input = allow.UpdatedInput
		}
		if len(allow.UpdatedPermissions) > 0 {
			e.ApplyPermissionUpdates(allow.UpdatedPermissions)
		}
	}

	if e.opts == nil {
		return "", fmt.Errorf("tool execution options are nil")
	}

	cfg, ok := e.opts.MCPServers[ref.Server]
	if !ok {
		return "", fmt.Errorf("server not configured for tool: %s", name)
	}
	switch sc := cfg.(type) {
	case *mcp.SdkServerConfig:
		if sc == nil || sc.Instance == nil {
			return "", fmt.Errorf("invalid MCP server instance for tool: %s", name)
		}
		inst, ok := sc.Instance.(mcp.ServerInstance)
		if !ok {
			return "", fmt.Errorf("invalid MCP server instance for tool: %s", name)
		}

		result, err := inst.CallTool(ctx, ref.Tool, input)
		if err != nil {
			return "", err
		}

		if txt, ok := extractText(result); ok {
			return txt, nil
		}

		b, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("marshal tool result: %w", err)
		}
		return string(b), nil

	case *mcp.StdioServerConfig, *mcp.SSEServerConfig, *mcp.HTTPServerConfig:
		result, err := externalCallTool(ctx, cfg, ref.Tool, input)
		if err != nil {
			return "", err
		}
		return formatExternalToolResult(result)

	default:
		return "", fmt.Errorf("unsupported MCP server type for tool: %s", name)
	}
}

func (e *Executor) evaluatePolicy(toolName string) (deny bool, reason string, interrupt bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.mode != nil {
		switch *e.mode {
		case permission.ModeBypassPermissions:
			return false, "", false
		}
	}

	behavior, ok := e.rules[toolName]
	if !ok {
		return false, "", false
	}
	switch behavior {
	case permission.BehaviorAllow:
		return false, "", false
	case permission.BehaviorDeny:
		return true, "tool denied by updated permission rules", false
	default:
		return false, "", false
	}
}

// SetMode updates the active permission mode for subsequent tool evaluations.
func (e *Executor) SetMode(mode permission.Mode) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.mode = &mode
}

// ApplyPermissionUpdates updates in-memory permission policy state.
func (e *Executor) ApplyPermissionUpdates(updates []*permission.Update) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, up := range updates {
		if up == nil {
			continue
		}
		switch up.Type {
		case permission.UpdateTypeSetMode:
			e.mode = up.Mode
		case permission.UpdateTypeAddRules, permission.UpdateTypeReplaceRules:
			if up.Type == permission.UpdateTypeReplaceRules {
				e.rules = make(map[string]permission.Behavior, len(up.Rules))
			}
			behavior := permission.BehaviorAsk
			if up.Behavior != nil {
				behavior = *up.Behavior
			}
			for _, r := range up.Rules {
				if r == nil || r.ToolName == "" {
					continue
				}
				e.rules[r.ToolName] = behavior
			}
		case permission.UpdateTypeRemoveRules:
			for _, r := range up.Rules {
				if r == nil || r.ToolName == "" {
					continue
				}
				delete(e.rules, r.ToolName)
			}
		}
		if e.store != nil {
			_ = e.store.ApplyPersistentUpdate(up)
		}
	}
}

func extractText(result map[string]any) (string, bool) {
	c, ok := result["content"]
	if !ok {
		return "", false
	}
	arr, ok := c.([]map[string]any)
	if ok {
		for _, item := range arr {
			if item["type"] == "text" {
				if t, ok := item["text"].(string); ok {
					return t, true
				}
			}
		}
	}
	// tolerate []any shape
	if anyArr, ok := c.([]any); ok {
		for _, it := range anyArr {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == "text" {
				if t, ok := m["text"].(string); ok {
					return t, true
				}
			}
		}
	}
	return "", false
}

func formatExternalToolResult(result *gomcp.CallToolResult) (string, error) {
	if result == nil {
		return "", nil
	}
	for _, c := range result.Content {
		if tc, ok := c.(*gomcp.TextContent); ok {
			return tc.Text, nil
		}
	}
	if result.StructuredContent != nil {
		b, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return "", fmt.Errorf("marshal external structured content: %w", err)
		}
		return string(b), nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("marshal external tool result: %w", err)
	}
	return string(b), nil
}
