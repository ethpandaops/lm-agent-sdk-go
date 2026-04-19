package lmsdk

import (
	"context"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/lmstudio"
)

// ListModels returns the available vLLM-served models.
func ListModels(ctx context.Context, opts ...Option) ([]ModelInfo, error) {
	resp, err := ListModelsResponse(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return resp.Models, nil
}

// ListModelsResponse returns the full vLLM model discovery payload.
func ListModelsResponse(ctx context.Context, opts ...Option) (*ModelListResponse, error) {
	cfg := applyAgentOptions(opts)
	if cfg == nil {
		cfg = &config.Options{}
	}
	cfg.ApplyDefaults()
	return lmstudio.ListModelsResponse(ctx, cfg)
}
