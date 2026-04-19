package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
)

func TestRegistryIncludesExternalToolsViaLister(t *testing.T) {
	old := externalListTools
	defer func() { externalListTools = old }()

	externalListTools = func(cfg mcp.ServerConfig) ([]externalListedTool, error) {
		return []externalListedTool{
			{
				Name:   "search",
				Desc:   "search",
				Schema: map[string]any{"type": "object"},
			},
		}, nil
	}

	opts := &config.Options{
		MCPServers: map[string]mcp.ServerConfig{
			"ext": &mcp.HTTPServerConfig{
				Type: mcp.ServerTypeHTTP,
				URL:  "http://example",
			},
		},
	}
	reg := NewRegistry(opts)

	tools := reg.OpenAITools()
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	fn := tools[0]["function"].(map[string]any)
	if fn["name"] != "mcp__ext__search" {
		t.Fatalf("unexpected function name: %#v", fn["name"])
	}

	status := reg.ServerStatuses()["ext"]
	if status.Status != "ready" {
		t.Fatalf("expected ext status ready, got %+v", status)
	}
	if status.Reason != "" {
		t.Fatalf("expected empty ready reason, got %+v", status)
	}
}

func TestRegistrySetsExternalErrorStatusWhenListingFails(t *testing.T) {
	old := externalListTools
	defer func() { externalListTools = old }()

	externalListTools = func(cfg mcp.ServerConfig) ([]externalListedTool, error) {
		return nil, errors.New("connect failed")
	}

	opts := &config.Options{
		MCPServers: map[string]mcp.ServerConfig{
			"ext": &mcp.SSEServerConfig{
				Type: mcp.ServerTypeSSE,
				URL:  "http://example/sse",
			},
		},
	}
	reg := NewRegistry(opts)

	tools := reg.OpenAITools()
	if len(tools) != 0 {
		t.Fatalf("expected no tools when listing fails, got %d", len(tools))
	}
	status := reg.ServerStatuses()["ext"]
	if status.Status != "error" {
		t.Fatalf("expected ext error status, got %+v", status)
	}
	if status.Reason == "" {
		t.Fatalf("expected failure reason, got %+v", status)
	}
}

func TestExecutorCallsExternalTool(t *testing.T) {
	oldList := externalListTools
	oldCall := externalCallTool
	defer func() {
		externalListTools = oldList
		externalCallTool = oldCall
	}()

	externalListTools = func(cfg mcp.ServerConfig) ([]externalListedTool, error) {
		return []externalListedTool{
			{Name: "search", Desc: "search", Schema: map[string]any{"type": "object"}},
		}, nil
	}
	externalCallTool = func(ctx context.Context, cfg mcp.ServerConfig, toolName string, input map[string]any) (*gomcp.CallToolResult, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{
				&gomcp.TextContent{Text: "external-ok"},
			},
		}, nil
	}

	opts := &config.Options{
		MCPServers: map[string]mcp.ServerConfig{
			"ext": &mcp.SSEServerConfig{
				Type: mcp.ServerTypeSSE,
				URL:  "http://example/sse",
			},
		},
	}
	reg := NewRegistry(opts)
	exec := NewExecutor(opts, reg)
	out, err := exec.Execute(context.Background(), "mcp__ext__search", map[string]any{"q": "hello"})
	if err != nil {
		t.Fatalf("execute external: %v", err)
	}
	if out != "external-ok" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestWithExternalRetryRetriesTransientErrors(t *testing.T) {
	oldBackoff := externalBackoff
	externalBackoff = func(int) time.Duration { return 0 }
	defer func() { externalBackoff = oldBackoff }()

	attempts := 0
	got, err := withExternalRetry(context.Background(), 3, func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", errors.New("connection reset by peer")
		}
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("unexpected result: %q", got)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestWithExternalRetryStopsOnNonRetryableError(t *testing.T) {
	attempts := 0
	_, err := withExternalRetry(context.Background(), 3, func(context.Context) (string, error) {
		attempts++
		return "", errors.New("invalid stdio MCP config")
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt for non-retryable error, got %d", attempts)
	}
}
