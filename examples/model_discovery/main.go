package main

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	resp, err := sdk.ListModelsResponse(ctx, sdk.WithAPIKey(exampleutil.APIKey()))
	if err != nil {
		fmt.Printf("list models error: %v\n", err)
		return
	}

	fmt.Printf("Endpoint: %s (authenticated=%v)\n", resp.Endpoint, resp.Authenticated)
	fmt.Printf("Models discovered: %d\n", resp.Total)

	shown := 0
	for _, m := range resp.Models {
		if !m.SupportsToolCalling() && !m.IsFree {
			continue
		}
		fmt.Printf("- %s | tier=%s | ctx=%d | tools=%v | structured=%v | reasoning=%v | image-in=%v | image-out=%v\n",
			m.ID, m.CostTier(), m.MaxContextLength(), m.SupportsToolCalling(), m.SupportsStructuredOutput(), m.SupportsReasoning(), m.SupportsImageInput(), m.SupportsImageOutput())
		shown++
		if shown == 5 {
			break
		}
	}
}
