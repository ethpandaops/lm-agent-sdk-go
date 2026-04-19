package message

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Message represents any message in the conversation.
// Use type assertion or type switch to determine the concrete type.
type Message interface {
	MessageType() string
}

// Compile-time verification that all message types implement Message.
var (
	_ Message = (*UserMessage)(nil)
	_ Message = (*AssistantMessage)(nil)
	_ Message = (*SystemMessage)(nil)
	_ Message = (*ResultMessage)(nil)
	_ Message = (*StreamEvent)(nil)
)

const rawJSONKey = "\x00sdk_raw_json"

// AuditEnvelope contains the provider-native event metadata and canonical payload
// captured at the SDK boundary.
type AuditEnvelope struct {
	EventType string          `json:"event_type"`
	Subtype   string          `json:"subtype,omitempty"`
	Payload   json.RawMessage `json:"payload"`
}

func (a *AuditEnvelope) GetEventType() string {
	if a == nil {
		return ""
	}

	return a.EventType
}

func (a *AuditEnvelope) GetSubtype() string {
	if a == nil {
		return ""
	}

	return a.Subtype
}

func (a *AuditEnvelope) GetPayload() json.RawMessage {
	if a == nil {
		return nil
	}

	return a.Payload
}

// AnnotateRawJSON attaches the original raw JSON bytes to a decoded payload so
// audit envelopes can preserve byte fidelity later in the pipeline.
func AnnotateRawJSON(data map[string]any, raw []byte) map[string]any {
	if data == nil || len(raw) == 0 {
		return data
	}

	data[rawJSONKey] = append([]byte(nil), raw...)

	return data
}

func extractRawJSON(data map[string]any) (json.RawMessage, bool) {
	if data == nil {
		return nil, false
	}

	switch raw := data[rawJSONKey].(type) {
	case []byte:
		if len(raw) == 0 {
			return nil, false
		}

		return append(json.RawMessage(nil), raw...), true
	case json.RawMessage:
		if len(raw) == 0 {
			return nil, false
		}

		return append(json.RawMessage(nil), raw...), true
	case string:
		if raw == "" {
			return nil, false
		}

		return json.RawMessage(raw), true
	default:
		return nil, false
	}
}

func stripRawJSON(data map[string]any) map[string]any {
	if data == nil {
		return nil
	}

	if _, ok := data[rawJSONKey]; !ok {
		return data
	}

	sanitized := make(map[string]any, len(data)-1)
	for key, value := range data {
		if key == rawJSONKey {
			continue
		}

		sanitized[key] = value
	}

	return sanitized
}

// NewAuditEnvelope marshals a canonical payload into an audit envelope.
func NewAuditEnvelope(eventType, subtype string, payload any) (*AuditEnvelope, error) {
	var (
		data json.RawMessage
		err  error
	)

	switch typed := payload.(type) {
	case map[string]any:
		if raw, ok := extractRawJSON(typed); ok {
			data = raw
			break
		}

		data, err = json.Marshal(stripRawJSON(typed))
	case []byte:
		data = append(json.RawMessage(nil), typed...)
	case json.RawMessage:
		data = append(json.RawMessage(nil), typed...)
	default:
		data, err = json.Marshal(payload)
	}
	if err != nil {
		return nil, fmt.Errorf("marshal audit payload: %w", err)
	}

	return &AuditEnvelope{
		EventType: eventType,
		Subtype:   subtype,
		Payload:   data,
	}, nil
}

// UserMessageContent represents content that can be either a string or []ContentBlock.
type UserMessageContent struct {
	text   *string        // Set when content is a string
	blocks []ContentBlock // Set when content is array of blocks
}

// NewUserMessageContent creates UserMessageContent from a string.
func NewUserMessageContent(text string) UserMessageContent {
	return UserMessageContent{text: &text}
}

// NewUserMessageContentBlocks creates UserMessageContent from blocks.
func NewUserMessageContentBlocks(blocks []ContentBlock) UserMessageContent {
	return UserMessageContent{blocks: blocks}
}

// String returns the string content if it was originally a string, or the concatenated text blocks.
func (c *UserMessageContent) String() string {
	if c.text != nil {
		return *c.text
	}
	if len(c.blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(c.blocks))
	for _, block := range c.blocks {
		if text, ok := block.(*TextBlock); ok && strings.TrimSpace(text.Text) != "" {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// Blocks returns content as []ContentBlock (normalizes string to TextBlock).
func (c *UserMessageContent) Blocks() []ContentBlock {
	if c.blocks != nil {
		return c.blocks
	}

	if c.text != nil {
		return []ContentBlock{
			&TextBlock{Type: "text", Text: *c.text},
		}
	}

	return nil
}

// IsString returns true if content was originally a string.
func (c *UserMessageContent) IsString() bool {
	return c.text != nil
}

// HasBlocks reports whether the content is represented as a block array.
func (c *UserMessageContent) HasBlocks() bool {
	return len(c.blocks) > 0
}

// HasNonTextBlocks reports whether the content contains multimodal input blocks.
func (c *UserMessageContent) HasNonTextBlocks() bool {
	for _, block := range c.Blocks() {
		if _, ok := block.(*TextBlock); !ok {
			return true
		}
	}
	return false
}

// MarshalJSON implements json.Marshaler.
// Outputs string if content is string, otherwise outputs array of blocks.
func (c UserMessageContent) MarshalJSON() ([]byte, error) {
	if c.text != nil {
		return json.Marshal(*c.text)
	}

	return json.Marshal(c.blocks)
}

// UnmarshalJSON implements json.Unmarshaler.
// Accepts both string and array of content blocks.
func (c *UserMessageContent) UnmarshalJSON(data []byte) error {
	// Try string first
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.text = &text
		c.blocks = nil

		return nil
	}

	// Try array of blocks
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(data, &rawBlocks); err != nil {
		return err
	}

	blocks := make([]ContentBlock, 0, len(rawBlocks))

	for _, raw := range rawBlocks {
		block, err := UnmarshalContentBlock(raw)
		if err != nil {
			return err
		}

		blocks = append(blocks, block)
	}

	c.blocks = blocks
	c.text = nil

	return nil
}

// UserMessage represents a message from the user.
//
//nolint:tagliatelle // Legacy compatibility payloads use snake_case.
type UserMessage struct {
	Type            string             `json:"type"`
	Content         UserMessageContent `json:"content"`
	UUID            *string            `json:"uuid,omitempty"`
	ParentToolUseID *string            `json:"parent_tool_use_id,omitempty"`
	ToolUseResult   map[string]any     `json:"tool_use_result,omitempty"`
	Audit           *AuditEnvelope     `json:"-"`
}

// MessageType implements the Message interface.
func (m *UserMessage) MessageType() string { return "user" }

// AssistantMessage represents a message from the model.
//
//nolint:tagliatelle // Legacy compatibility payloads use snake_case.
type AssistantMessage struct {
	Type            string                 `json:"type"`
	Content         []ContentBlock         `json:"content"`
	Model           string                 `json:"model"`
	ParentToolUseID *string                `json:"parent_tool_use_id,omitempty"`
	Error           *AssistantMessageError `json:"error,omitempty"`
	Audit           *AuditEnvelope         `json:"-"`
}

// MessageType implements the Message interface.
func (m *AssistantMessage) MessageType() string { return "assistant" }

// AssistantMessageError represents error types from the assistant.
type AssistantMessageError string

const (
	// AssistantMessageErrorAuthFailed indicates authentication failure.
	AssistantMessageErrorAuthFailed AssistantMessageError = "authentication_failed"
	// AssistantMessageErrorBilling indicates a billing error.
	AssistantMessageErrorBilling AssistantMessageError = "billing_error"
	// AssistantMessageErrorRateLimit indicates rate limiting.
	AssistantMessageErrorRateLimit AssistantMessageError = "rate_limit"
	// AssistantMessageErrorInvalidReq indicates an invalid request.
	AssistantMessageErrorInvalidReq AssistantMessageError = "invalid_request"
	// AssistantMessageErrorServer indicates a server error.
	AssistantMessageErrorServer AssistantMessageError = "server_error"
	// AssistantMessageErrorUnknown indicates an unknown error.
	AssistantMessageErrorUnknown AssistantMessageError = "unknown"
)

// SystemMessage represents a system message.
type SystemMessage struct {
	Type    string         `json:"type"`
	Subtype string         `json:"subtype,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	Audit   *AuditEnvelope `json:"-"`
}

// MessageType implements the Message interface.
func (m *SystemMessage) MessageType() string { return "system" }

// ResultMessage represents the final result of a query.
//
//nolint:tagliatelle // Legacy compatibility payloads use snake_case.
type ResultMessage struct {
	Type             string         `json:"type"`
	Subtype          string         `json:"subtype"`
	DurationMs       int            `json:"duration_ms"`
	DurationAPIMs    int            `json:"duration_api_ms"`
	IsError          bool           `json:"is_error"`
	NumTurns         int            `json:"num_turns"`
	SessionID        string         `json:"session_id"`
	TotalCostUSD     *float64       `json:"total_cost_usd,omitempty"`
	Usage            *Usage         `json:"usage,omitempty"`
	Result           *string        `json:"result,omitempty"`
	StopReason       *string        `json:"stop_reason,omitempty"`
	StructuredOutput any            `json:"structured_output,omitempty"`
	Audit            *AuditEnvelope `json:"-"`
}

// MessageType implements the Message interface.
func (m *ResultMessage) MessageType() string { return "result" }

// StreamEvent represents a raw streaming event payload.
//
//nolint:tagliatelle // Legacy compatibility payloads use snake_case.
type StreamEvent struct {
	UUID            string         `json:"uuid"`
	SessionID       string         `json:"session_id"`
	Event           map[string]any `json:"event"` // Raw Anthropic API event
	ParentToolUseID *string        `json:"parent_tool_use_id,omitempty"`
	Audit           *AuditEnvelope `json:"-"`
}

// MessageType implements the Message interface.
func (m *StreamEvent) MessageType() string { return "stream_event" }

// Usage contains token usage information.
//
//nolint:tagliatelle // Legacy compatibility payloads use snake_case.
type Usage struct {
	InputTokens           int `json:"input_tokens"`
	OutputTokens          int `json:"output_tokens"`
	CachedInputTokens     int `json:"cached_input_tokens"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens"`
}

// StreamingMessageContent represents the content of a streaming message.
type StreamingMessageContent struct {
	Role    string             `json:"role"`    // "user"
	Content UserMessageContent `json:"content"` // The message content
}

// StreamingMessage represents a message in the SDK's streaming input format.
//
//nolint:tagliatelle // Legacy compatibility payloads use snake_case fields.
type StreamingMessage struct {
	Type            string                  `json:"type"`                         // "user"
	Message         StreamingMessageContent `json:"message"`                      // The message content
	ParentToolUseID *string                 `json:"parent_tool_use_id,omitempty"` // Optional parent tool use ID
	SessionID       string                  `json:"session_id,omitempty"`         // Optional session ID
}
