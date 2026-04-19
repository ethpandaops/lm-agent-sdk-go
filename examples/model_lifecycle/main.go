// Package main demonstrates LM Studio's /api/v1/models lifecycle surface —
// read-only operations (ListModelsResponse) only, so this example is safe
// to run without disturbing the currently-loaded model.
//
// For mutating operations — LoadModel, UnloadModel, DownloadModel — see
// the corresponding function docstrings in native.go. Mutating calls are
// not demoed here because they can evict whatever model you have loaded.
package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := sdk.ListModelsResponse(ctx, sdk.WithAPIKey(exampleutil.APIKey()))
	if err != nil {
		fmt.Printf("list error: %v\n", err)
		return
	}

	fmt.Printf("Endpoint: %s  (authenticated=%v)\n", resp.Endpoint, resp.Authenticated)
	fmt.Printf("Models discovered: %d\n\n", len(resp.Models))

	for _, m := range resp.Models {
		caps := strings.Join(m.Capabilities, ",")
		if caps == "" {
			caps = "-"
		}
		fmt.Printf("  %-45s type=%-10s state=%-10s quant=%-8s caps=%s\n",
			m.ID, m.Architecture.Modality, m.State, m.Quantization, caps)
	}

	fmt.Println()
	fmt.Println("To load/unload/download a model programmatically:")
	fmt.Println("  sdk.LoadModel(ctx, sdk.LoadRequest{Model: \"<id>\", LoadConfig: sdk.LoadConfig{...}})")
	fmt.Println("  sdk.UnloadModel(ctx, \"<instance_id>\")")
	fmt.Println("  sdk.DownloadModel(ctx, sdk.DownloadRequest{Model: \"<catalog or HF ref>\"})")
}
