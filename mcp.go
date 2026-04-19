package lmsdk

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	internalmcp "github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
)

// Tool represents a custom tool that the SDK can invoke through VLLM tool calling.
//
// Tools allow users to extend agent capabilities with domain-specific
// functionality. When registered, the model can discover and execute these
// tools during a session.
//
// Example:
//
//	tool := lmsdk.NewTool(
//	    "calculator",
//	    "Performs basic arithmetic operations",
//	    map[string]any{
//	        "type": "object",
//	        "properties": map[string]any{
//	            "operation": map[string]any{
//	                "type": "string",
//	                "enum": []string{"add", "subtract", "multiply", "divide"},
//	            },
//	            "a": map[string]any{"type": "number"},
//	            "b": map[string]any{"type": "number"},
//	        },
//	        "required": []string{"operation", "a", "b"},
//	    },
//	    func(ctx context.Context, input map[string]any) (map[string]any, error) {
//	        op := input["operation"].(string)
//	        a := input["a"].(float64)
//	        b := input["b"].(float64)
//
//	        var result float64
//	        switch op {
//	        case "add":
//	            result = a + b
//	        case "subtract":
//	            result = a - b
//	        case "multiply":
//	            result = a * b
//	        case "divide":
//	            if b == 0 {
//	                return nil, fmt.Errorf("division by zero")
//	            }
//	            result = a / b
//	        }
//
//	        return map[string]any{"result": result}, nil
//	    },
//	)
type Tool interface {
	// Name returns the unique identifier for this tool.
	Name() string

	// Description returns a human-readable description for the model.
	Description() string

	// InputSchema returns a JSON schema describing expected input.
	// The schema should follow JSON Schema Draft 7 specification.
	InputSchema() map[string]any

	// Execute runs the tool with the provided input.
	// The input will be validated against InputSchema before execution.
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

// ToolFunc is a function-based tool implementation.
type ToolFunc func(ctx context.Context, input map[string]any) (map[string]any, error)

// NewTool creates a Tool from a function.
//
// This is a convenience constructor for creating tools without implementing
// the full Tool interface.
//
// Parameters:
//   - name: Unique identifier for the tool (e.g., "calculator", "search_database")
//   - description: Human-readable description of what the tool does
//   - schema: JSON Schema defining the expected input structure
//   - fn: Function that executes the tool logic
func NewTool(name, description string, schema map[string]any, fn ToolFunc) Tool {
	return &tool{
		name:        name,
		description: description,
		schema:      schema,
		fn:          fn,
	}
}

// tool is the internal tool implementation.
type tool struct {
	name        string
	description string
	schema      map[string]any
	fn          ToolFunc
}

// Compile-time verification that *tool implements the Tool interface.
var _ Tool = (*tool)(nil)

func (t *tool) Name() string                { return t.name }
func (t *tool) Description() string         { return t.description }
func (t *tool) InputSchema() map[string]any { return t.schema }
func (t *tool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	return t.fn(ctx, input)
}

// createSDKToolServer wraps high-level Tool instances into an in-process MCP server config.
func createSDKToolServer(tools []Tool) *MCPSdkServerConfig {
	server := internalmcp.NewSDKServer("sdk", "1.0.0")

	for _, t := range tools {
		schema := mapToJSONSchema(t.InputSchema())
		mcpTool := internalmcp.NewTool(t.Name(), t.Description(), schema)
		server.AddTool(mcpTool, toolToMCPHandler(t))
	}

	return &MCPSdkServerConfig{
		Type:     MCPServerTypeSDK,
		Name:     "sdk",
		Instance: server,
	}
}

// toolToMCPHandler adapts a high-level Tool.Execute to an mcp.ToolHandler.
func toolToMCPHandler(t Tool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := internalmcp.ParseArguments(req)
		if err != nil {
			return internalmcp.ErrorResult(fmt.Sprintf("failed to parse arguments: %v", err)), nil
		}

		result, err := t.Execute(ctx, args)
		if err != nil {
			return internalmcp.ErrorResult(err.Error()), nil
		}

		data, err := json.Marshal(result)
		if err != nil {
			return internalmcp.ErrorResult(fmt.Sprintf("failed to marshal result: %v", err)), nil
		}

		return internalmcp.TextResult(string(data)), nil
	}
}

func mapToJSONSchema(m map[string]any) *jsonschema.Schema {
	if m == nil {
		return nil
	}

	data, err := json.Marshal(m)
	if err != nil {
		return nil
	}

	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil
	}

	return &schema
}
