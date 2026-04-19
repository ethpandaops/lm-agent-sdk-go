package lmsdk

import (
	"context"
	"iter"
)

// Client provides an interactive, stateful interface for multi-turn LM Studio conversations.
//
// Unlike the one-shot Query() function, Client maintains session state across
// multiple exchanges. It supports interruption, tool loops, and local session state.
//
// Lifecycle: clients are single-use. After Close(), create a new client with NewClient().
//
// Example usage:
//
//	client := NewClient()
//	defer func() { _ = client.Close() }()
//
//	err := client.Start(ctx,
//	    WithLogger(slog.Default()),
//	    WithPermissionMode("acceptEdits"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Send a query
//	err = client.Query(ctx, NewUserMessageContent("What is 2+2?"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Receive all messages for this response (stops at ResultMessage)
//	for msg, err := range client.ReceiveResponse(ctx) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    // Process message...
//	}
//
//	// Or receive messages indefinitely (for continuous streaming)
//	for msg, err := range client.ReceiveMessages(ctx) {
//	    if err != nil {
//	        break
//	    }
//	    // Process message...
//	}
type Client interface {
	// Start initializes the client runtime.
	// Must be called before any other methods.
	// Returns a transport/runtime error on failure.
	Start(ctx context.Context, opts ...Option) error

	// StartWithContent initializes the runtime and immediately sends initial user content.
	// Equivalent to calling Start() followed by Query(ctx, content).
	// The content is sent to the "default" session.
	// Returns a transport/runtime error on failure.
	StartWithContent(ctx context.Context, content UserMessageContent, opts ...Option) error

	// StartWithStream initializes the runtime and consumes the provided input iterator
	// as the initial message stream for the active session.
	// The iterator is consumed in a separate goroutine; use context cancellation to abort.
	// Returns a transport/runtime error on failure.
	StartWithStream(ctx context.Context, messages iter.Seq[StreamingMessage], opts ...Option) error

	// Query sends user content to the active session.
	// Returns immediately after sending; use ReceiveMessages() or ReceiveResponse() to get responses.
	// Optional sessionID defaults to "default" for multi-session support.
	Query(ctx context.Context, content UserMessageContent, sessionID ...string) error

	// ReceiveMessages returns an iterator that yields messages indefinitely.
	// Messages are yielded as they arrive until EOF, an error occurs, or context is cancelled.
	// Unlike ReceiveResponse, this iterator does not stop at ResultMessage.
	// Use iter.Pull2 if you need pull-based iteration instead of range.
	ReceiveMessages(ctx context.Context) iter.Seq2[Message, error]

	// ReceiveResponse returns an iterator that yields messages until a ResultMessage is received.
	// Messages are yielded as they arrive for streaming consumption.
	// The iterator stops after yielding the ResultMessage.
	// Use iter.Pull2 if you need pull-based iteration instead of range.
	// To collect all messages into a slice, use slices.Collect or a simple loop.
	ReceiveResponse(ctx context.Context) iter.Seq2[Message, error]

	// Interrupt cancels the current in-flight request.
	Interrupt(ctx context.Context) error

	// SetPermissionMode changes the permission mode during conversation.
	// Valid modes: "default", "acceptEdits", "plan", "bypassPermissions"
	SetPermissionMode(ctx context.Context, mode string) error

	// SetModel changes the AI model during conversation.
	// Pass nil to use the default model.
	SetModel(ctx context.Context, model *string) error

	// ListModels returns the available LM Studio models using the current client options.
	ListModels(ctx context.Context) ([]ModelInfo, error)

	// ListModelsResponse returns the full LM Studio model discovery payload.
	ListModelsResponse(ctx context.Context) (*ModelListResponse, error)

	// GetServerInfo returns runtime metadata for the active client session.
	// Returns nil when the client is not connected.
	GetServerInfo() map[string]any

	// GetMCPStatus returns MCP server connection status for the current runtime.
	// Returns the status of all configured MCP servers.
	GetMCPStatus(ctx context.Context) (*MCPStatus, error)

	// ReconnectMCPServer reconnects a disconnected or failed MCP server.
	ReconnectMCPServer(ctx context.Context, serverName string) error

	// ToggleMCPServer enables or disables an MCP server.
	ToggleMCPServer(ctx context.Context, serverName string, enabled bool) error

	// StopTask stops a running task by task ID.
	StopTask(ctx context.Context, taskID string) error

	// RewindFiles rewinds tracked files to their state at a specific user message.
	// The userMessageID should be the ID of a previous user message in the conversation.
	// Requires EnableFileCheckpointing=true in AgentOptions.
	RewindFiles(ctx context.Context, userMessageID string) error

	// SendToolResult sends a tool result for a pending tool call.
	SendToolResult(ctx context.Context, toolUseID, content string, isError bool) error

	// Close terminates the session and cleans up resources.
	// After Close(), the client cannot be reused. Safe to call multiple times.
	Close() error
}

// NewClient creates a new interactive client.
//
// Call Start() with options to begin a session:
//
//	client := NewClient()
//	err := client.Start(ctx,
//	    WithLogger(slog.Default()),
//	    WithPermissionMode("acceptEdits"),
//	)
func NewClient() Client {
	return newClientImpl()
}
