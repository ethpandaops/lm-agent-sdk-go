package config

import "context"

// ChatRequest is the normalized chat completion request used by transports.
type ChatRequest struct {
	Model              string
	Messages           []map[string]any
	Tools              []map[string]any
	Stream             bool
	ToolChoice         any
	MaxTokens          *int
	Temperature        *float64
	TopP               *float64
	TopK               *float64
	PresencePenalty    *float64
	FrequencyPenalty   *float64
	Seed               *int64
	Stop               []string
	Logprobs           *bool
	TopLogprobs        *int
	ParallelToolCalls  *bool
	ResponseFormat     map[string]any
	Reasoning          map[string]any
	User               string
	MaxToolCalls       *int
	StreamIncludeUsage *bool
	Extra              map[string]any

	// LM Studio-specific extensions
	TTL           *int
	DraftModel    string
	MinP          *float64
	RepeatPenalty *float64
}

// Transport defines the runtime transport interface.
type Transport interface {
	Start(ctx context.Context) error
	CreateStream(ctx context.Context, req *ChatRequest) (<-chan map[string]any, <-chan error)
	Close() error
}
