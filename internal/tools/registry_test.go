package tools

import (
	"context"
	"testing"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
)

type multiToolServer struct{}

func (s *multiToolServer) Name() string    { return "multi" }
func (s *multiToolServer) Version() string { return "1.0.0" }
func (s *multiToolServer) ListTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "echo",
			"description": "echo",
			"inputSchema": map[string]any{"type": "object"},
		},
		{
			"name":        "sum",
			"description": "sum",
			"inputSchema": map[string]any{"type": "object"},
		},
	}
}
func (s *multiToolServer) CallTool(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

func TestRegistryRespectsToolsList(t *testing.T) {
	opts := &config.Options{
		Tools: config.ToolsList{"mcp__srv__echo"},
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &multiToolServer{}},
		},
	}
	reg := NewRegistry(opts)
	tools := reg.OpenAITools()
	if len(tools) != 1 {
		t.Fatalf("expected exactly one tool from ToolsList filter, got %d", len(tools))
	}
	fn := tools[0]["function"].(map[string]any)
	if fn["name"] != "mcp__srv__echo" {
		t.Fatalf("expected only echo tool, got %#v", fn["name"])
	}
}

func TestRegistryToolsListIntersectsWithAllowedTools(t *testing.T) {
	opts := &config.Options{
		Tools:        config.ToolsList{"mcp__srv__echo"},
		AllowedTools: []string{"mcp__srv__sum"},
		MCPServers: map[string]mcp.ServerConfig{
			"srv": &mcp.SdkServerConfig{Type: mcp.ServerTypeSDK, Name: "srv", Instance: &multiToolServer{}},
		},
	}
	reg := NewRegistry(opts)
	tools := reg.OpenAITools()
	if len(tools) != 0 {
		t.Fatalf("expected empty intersection between ToolsList and AllowedTools, got %d tools", len(tools))
	}
}
