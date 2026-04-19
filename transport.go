package lmsdk

import (
	"context"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
)

// ChatRequest is the normalized request sent through a Transport.
type ChatRequest = config.ChatRequest

// Transport defines the runtime transport interface.
type Transport interface {
	Start(ctx context.Context) error
	CreateStream(ctx context.Context, req *ChatRequest) (<-chan map[string]any, <-chan error)
	Close() error
}
