package errors

import (
	"errors"
	"fmt"
)

// SDKError is the base interface for all SDK errors.
type SDKError interface {
	error
	IsSDKError() bool
}

// Compile-time verification that all error types implement SDKError.
var (
	_ SDKError = (*MessageParseError)(nil)
	_ SDKError = (*ToolPermissionDeniedError)(nil)
	_ SDKError = (*UnsupportedHookEventError)(nil)
	_ SDKError = (*UnsupportedHookOutputError)(nil)
)

// Sentinel errors for commonly checked conditions.
var (
	// ErrClientNotConnected indicates the client is not connected.
	ErrClientNotConnected = errors.New("client not connected")

	// ErrClientAlreadyConnected indicates the client is already connected.
	ErrClientAlreadyConnected = errors.New("client already connected")

	// ErrClientClosed indicates the client has been closed and cannot be reused.
	ErrClientClosed = errors.New("client closed: clients are single-use, create a new one with New()")

	// ErrTransportNotConnected indicates the transport is not connected.
	ErrTransportNotConnected = errors.New("transport not connected")

	// ErrRequestTimeout indicates a request timed out.
	ErrRequestTimeout = errors.New("request timeout")

	// ErrSessionNotFound indicates a requested local session could not be found.
	ErrSessionNotFound = errors.New("session not found")

	// ErrControllerStopped indicates the protocol controller has stopped.
	ErrControllerStopped = errors.New("protocol controller stopped")

	// ErrStdinClosed indicates stdin was closed due to context cancellation.
	ErrStdinClosed = errors.New("stdin closed")

	// ErrOperationCancelled indicates an operation was cancelled via cancel request.
	ErrOperationCancelled = errors.New("operation cancelled")

	// ErrUnknownMessageType indicates the message type is not recognized by the SDK.
	// Callers should skip these messages rather than treating them as fatal.
	ErrUnknownMessageType = errors.New("unknown message type")
)

// MessageParseError indicates message parsing failed.
type MessageParseError struct {
	Message string
	Err     error
	Data    map[string]any
}

func (e *MessageParseError) Error() string {
	return fmt.Sprintf("failed to parse message: %v", e.Err)
}

func (e *MessageParseError) Unwrap() error {
	return e.Err
}

// IsSDKError implements SDKError.
func (e *MessageParseError) IsSDKError() bool { return true }

// ToolPermissionDeniedError indicates a tool was denied by permission policy.
type ToolPermissionDeniedError struct {
	ToolName  string
	Message   string
	Interrupt bool
}

func (e *ToolPermissionDeniedError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("tool denied: %s", e.ToolName)
}

// IsSDKError implements SDKError.
func (e *ToolPermissionDeniedError) IsSDKError() bool { return true }

// UnsupportedHookEventError indicates a hook event is not supported by this backend runtime.
type UnsupportedHookEventError struct {
	Event string
}

func (e *UnsupportedHookEventError) Error() string {
	return fmt.Sprintf("unsupported hook event in LM Studio backend: %s", e.Event)
}

// IsSDKError implements SDKError.
func (e *UnsupportedHookEventError) IsSDKError() bool { return true }

// UnsupportedHookOutputError indicates a hook output field is not supported by this backend runtime.
type UnsupportedHookOutputError struct {
	Event string
	Field string
}

func (e *UnsupportedHookOutputError) Error() string {
	if e.Field == "" {
		return fmt.Sprintf("unsupported hook output in LM Studio backend for event %s", e.Event)
	}
	return fmt.Sprintf("unsupported hook output field in LM Studio backend: %s.%s", e.Event, e.Field)
}

// IsSDKError implements SDKError.
func (e *UnsupportedHookOutputError) IsSDKError() bool { return true }
