package lmsdk

import (
	"errors"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/controlplane"
	internalerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/session"
)

// Re-export error types from internal package

// MessageParseError indicates message parsing failed.
type MessageParseError = internalerrors.MessageParseError

// ToolPermissionDeniedError indicates a tool execution was denied by permission policy.
type ToolPermissionDeniedError = internalerrors.ToolPermissionDeniedError

// UnsupportedHookEventError indicates a configured hook event is not supported by this backend.
type UnsupportedHookEventError = internalerrors.UnsupportedHookEventError

// UnsupportedHookOutputError indicates a hook output field is unsupported by this backend.
type UnsupportedHookOutputError = internalerrors.UnsupportedHookOutputError

// UnsupportedControlError indicates a control-plane operation is unsupported by this backend.
type UnsupportedControlError = controlplane.UnsupportedControlError

// SDKError is the base interface for all SDK errors.
type SDKError = internalerrors.SDKError

// Re-export sentinel errors from internal package.
var (
	// ErrClientNotConnected indicates the client is not connected.
	ErrClientNotConnected = internalerrors.ErrClientNotConnected

	// ErrClientAlreadyConnected indicates the client is already connected.
	ErrClientAlreadyConnected = internalerrors.ErrClientAlreadyConnected

	// ErrClientClosed indicates the client has been closed and cannot be reused.
	ErrClientClosed = internalerrors.ErrClientClosed

	// ErrTransportNotConnected indicates the transport is not connected.
	ErrTransportNotConnected = internalerrors.ErrTransportNotConnected

	// ErrRequestTimeout indicates a request timed out.
	ErrRequestTimeout = internalerrors.ErrRequestTimeout

	// ErrSessionNotFound indicates a requested local session does not exist.
	ErrSessionNotFound = internalerrors.ErrSessionNotFound

	// ErrUnsupportedFeature indicates an API-compatible feature that is not implemented by this backend.
	ErrUnsupportedFeature = errors.New("unsupported feature in LM Studio backend")

	// ErrUnsupportedControl indicates a control-plane operation is not supported by backend.
	ErrUnsupportedControl = controlplane.ErrUnsupportedControl

	// ErrNoCheckpoint indicates rewind was requested without an available checkpoint.
	ErrNoCheckpoint = session.ErrNoCheckpoint
)
