//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

func TestSDKTools_ReturnValue(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var executions atomic.Int32
	var returnedValue string
	tool := lmsdk.NewTool("get_value", "Return the value 42", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, func(_ context.Context, _ map[string]any) (map[string]any, error) {
		executions.Add(1)
		returnedValue = "42"
		return map[string]any{"value": 42}, nil
	})

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		lmsdk.WithToolChoice("auto"),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText("Invoke the get_value tool exactly once (it takes no arguments) and then answer: done.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveMessages(ctx) {
		}
	}()

	if err := waitForCondition(ctx, func() bool { return executions.Load() > 0 }); err != nil {
		t.Fatalf("wait for tool execution: %v", err)
	}
	_ = client.Interrupt(ctx)
	<-done

	if returnedValue != "42" {
		t.Fatal("expected tool to return 42")
	}
}

func TestSDKTools_MultiTool(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var addExecutions atomic.Int32
	addTool := lmsdk.NewTool("add", "Add two numbers", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "number"},
			"b": map[string]any{"type": "number"},
		},
		"required": []string{"a", "b"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		addExecutions.Add(1)
		a, _ := input["a"].(float64)
		b, _ := input["b"].(float64)
		return map[string]any{"result": a + b}, nil
	})

	var mulExecutions atomic.Int32
	mulTool := lmsdk.NewTool("multiply", "Multiply two numbers", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"a": map[string]any{"type": "number"},
			"b": map[string]any{"type": "number"},
		},
		"required": []string{"a", "b"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		mulExecutions.Add(1)
		a, _ := input["a"].(float64)
		b, _ := input["b"].(float64)
		return map[string]any{"result": a * b}, nil
	})

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts, lmsdk.WithSDKTools(addTool, mulTool))

	result := collectResult(t, lmsdk.Query(ctx, promptText("What is 3+4? Use the add tool."), opts...))
	if result.Result == nil {
		t.Fatal("expected result")
	}

	if addExecutions.Load() == 0 {
		t.Fatal("expected add tool to execute")
	}
}

func TestSDKTools_ErrorHandler(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	tool := lmsdk.NewTool("failing_tool", "Always fails", map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}, func(_ context.Context, _ map[string]any) (map[string]any, error) {
		return nil, fmt.Errorf("intentional failure")
	})

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithSDKTools(tool),
		lmsdk.WithToolChoice("auto"),
	)

	// Should still complete — the error is reported to the model, not to the caller.
	var sawResult bool
	for msg, err := range lmsdk.Query(ctx, promptText("Call failing_tool."), opts...) {
		if err != nil {
			// Error propagation is also acceptable behavior.
			return
		}
		if _, ok := msg.(*lmsdk.ResultMessage); ok {
			sawResult = true
		}
	}

	if !sawResult {
		t.Fatal("expected result even with failing tool")
	}
}

func TestMCPServer_SDKServer(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	var executions atomic.Int32
	addTool := lmsdk.NewSdkMcpTool("add", "Add two numbers",
		lmsdk.SimpleSchema(map[string]string{"a": "float64", "b": "float64"}),
		func(_ context.Context, req *lmsdk.CallToolRequest) (*lmsdk.CallToolResult, error) {
			args, err := lmsdk.ParseArguments(req)
			if err != nil {
				return lmsdk.ErrorResult(err.Error()), nil
			}
			executions.Add(1)
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return lmsdk.TextResult(fmt.Sprintf("%.0f", a+b)), nil
		},
		lmsdk.WithAnnotations(&lmsdk.McpToolAnnotations{
			ReadOnlyHint: true,
		}),
	)

	calcServer := lmsdk.CreateSdkMcpServer("calc", "1.0.0", addTool)

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithMCPServers(map[string]lmsdk.MCPServerConfig{
			"calc": calcServer,
		}),
		lmsdk.WithAllowedTools("mcp__calc__add"),
		// LM Studio only supports string tool_choice ("none"/"auto"/"required").
		// Some thinking-mode models emit tool calls inside reasoning_content
		// when forced with "required"; "auto" with a strong prompt is more
		// reliable across models.
		lmsdk.WithToolChoice("auto"),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Query(ctx, promptText("What is 3 + 4? Use the add tool.")); err != nil {
		t.Fatalf("query: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveMessages(ctx) {
		}
	}()

	if err := waitForCondition(ctx, func() bool { return executions.Load() > 0 }); err != nil {
		t.Fatalf("wait for tool execution: %v", err)
	}
	_ = client.Interrupt(ctx)
	<-done

	if executions.Load() == 0 {
		t.Fatal("expected MCP calculator tool to execute")
	}
}

func TestMCPStatus(t *testing.T) {
	ctx, cancel := integrationContext(t)
	defer cancel()

	addTool := lmsdk.NewSdkMcpTool("add", "Add", lmsdk.SimpleSchema(map[string]string{"a": "float64", "b": "float64"}),
		func(_ context.Context, req *lmsdk.CallToolRequest) (*lmsdk.CallToolResult, error) {
			args, _ := lmsdk.ParseArguments(req)
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return lmsdk.TextResult(fmt.Sprintf("%.0f", a+b)), nil
		},
	)
	server := lmsdk.CreateSdkMcpServer("status_test", "1.0.0", addTool)

	opts := append([]lmsdk.Option{}, integrationOptions()...)
	opts = append(opts,
		lmsdk.WithMCPServers(map[string]lmsdk.MCPServerConfig{
			"status_test": server,
		}),
		lmsdk.WithAllowedTools("mcp__status_test__add"),
	)

	client := lmsdk.NewClient()
	if err := client.Start(ctx, opts...); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	status, err := client.GetMCPStatus(ctx)
	if err != nil {
		t.Fatalf("get mcp status: %v", err)
	}

	if status == nil {
		t.Fatal("expected non-nil MCP status")
	}
}
