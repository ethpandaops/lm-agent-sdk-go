// Package main demonstrates how to create calculator tools using MCP servers.
//
// This example shows how to create an in-process MCP server with calculator
// tools using the SDK with the official MCP SDK types.
package main

import (
	"context"
	"fmt"
	"math"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func createCalculatorTools() []*sdk.SdkMcpTool {
	calcAnnotations := &sdk.McpToolAnnotations{
		ReadOnlyHint:   true,
		IdempotentHint: true,
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
		sdk.WithAnnotations(calcAnnotations),
	)

	subtractTool := sdk.NewSdkMcpTool(
		"subtract", "Subtract one number from another",
		sdk.SimpleSchema(map[string]string{"a": "float64", "b": "float64"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return sdk.TextResult(fmt.Sprintf("%v - %v = %v", a, b, a-b)), nil
		},
		sdk.WithAnnotations(calcAnnotations),
	)

	multiplyTool := sdk.NewSdkMcpTool(
		"multiply", "Multiply two numbers",
		sdk.SimpleSchema(map[string]string{"a": "float64", "b": "float64"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return sdk.TextResult(fmt.Sprintf("%v × %v = %v", a, b, a*b)), nil
		},
		sdk.WithAnnotations(calcAnnotations),
	)

	divideTool := sdk.NewSdkMcpTool(
		"divide", "Divide one number by another",
		sdk.SimpleSchema(map[string]string{"a": "float64", "b": "float64"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			if b == 0 {
				return sdk.ErrorResult("Error: Division by zero is not allowed"), nil
			}
			return sdk.TextResult(fmt.Sprintf("%v ÷ %v = %v", a, b, a/b)), nil
		},
		sdk.WithAnnotations(calcAnnotations),
	)

	sqrtTool := sdk.NewSdkMcpTool(
		"sqrt", "Calculate square root",
		sdk.SimpleSchema(map[string]string{"n": "float64"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			n, _ := args["n"].(float64)
			if n < 0 {
				return sdk.ErrorResult(fmt.Sprintf("Error: Cannot calculate square root of negative number %v", n)), nil
			}
			return sdk.TextResult(fmt.Sprintf("√%v = %v", n, math.Sqrt(n))), nil
		},
		sdk.WithAnnotations(calcAnnotations),
	)

	powerTool := sdk.NewSdkMcpTool(
		"power", "Raise a number to a power",
		sdk.SimpleSchema(map[string]string{"base": "float64", "exponent": "float64"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			base, _ := args["base"].(float64)
			exponent, _ := args["exponent"].(float64)
			return sdk.TextResult(fmt.Sprintf("%v^%v = %v", base, exponent, math.Pow(base, exponent))), nil
		},
		sdk.WithAnnotations(calcAnnotations),
	)

	return []*sdk.SdkMcpTool{addTool, subtractTool, multiplyTool, divideTool, sqrtTool, powerTool}
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	tools := createCalculatorTools()
	calculator := sdk.CreateSdkMcpServer("calc", "2.0.0", tools...)

	prompts := []string{
		"List your tools",
		"Calculate 15 + 27",
		"What is 100 divided by 7?",
		"Calculate the square root of 144",
		"What is 2 raised to the power of 8?",
		"Calculate 20 * 3 - 10",
	}

	for _, prompt := range prompts {
		fmt.Printf("\n%s\n", "==================================================")
		fmt.Printf("Prompt: %s\n", prompt)
		fmt.Printf("%s\n", "==================================================")

		client := sdk.NewClient()

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)

		if err := client.Start(ctx,
			sdk.WithAPIKey(exampleutil.APIKey()),
			sdk.WithModel(exampleutil.DefaultModel()),
			sdk.WithMCPServers(map[string]sdk.MCPServerConfig{
				"calc": calculator,
			}),
			sdk.WithAllowedTools(
				"mcp__calc__add",
				"mcp__calc__subtract",
				"mcp__calc__multiply",
				"mcp__calc__divide",
				"mcp__calc__sqrt",
				"mcp__calc__power",
			),
			sdk.WithMaxTurns(10),
		); err != nil {
			fmt.Printf("start error: %v\n", err)
			cancel()
			_ = client.Close()
			return
		}

		if err := client.Query(ctx, sdk.Text(prompt)); err != nil {
			fmt.Printf("query error: %v\n", err)
			cancel()
			_ = client.Close()
			return
		}

		for msg, err := range client.ReceiveResponse(ctx) {
			if err != nil {
				fmt.Printf("receive error: %v\n", err)
				cancel()
				_ = client.Close()
				return
			}
			exampleutil.DisplayMessage(msg)
		}

		cancel()
		_ = client.Close()
	}
}
