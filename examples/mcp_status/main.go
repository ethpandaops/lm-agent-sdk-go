// Package main demonstrates querying MCP server connection status.
//
// This example creates an in-process MCP server, starts a client with it
// configured, and queries the live connection status of all MCP servers.
package main

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	addTool := sdk.NewSdkMcpTool(
		"add", "Add two numbers",
		sdk.SimpleSchema(map[string]string{"a": "float64", "b": "float64"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return sdk.TextResult(fmt.Sprintf("%v + %v = %v", a, b, a+b)), nil
		},
	)

	calculator := sdk.CreateSdkMcpServer("calc", "1.0.0", addTool)

	client := sdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMCPServers(map[string]sdk.MCPServerConfig{
			"calc": calculator,
		}),
	); err != nil {
		fmt.Printf("start error: %v\n", err)
		return
	}
	defer func() { _ = client.Close() }()

	status, err := client.GetMCPStatus(ctx)
	if err != nil {
		fmt.Printf("get mcp status error: %v\n", err)
		return
	}

	fmt.Println("MCP Server Status:")
	for _, server := range status.MCPServers {
		fmt.Printf("  %s: %s\n", server.Name, server.Status)
	}
}
