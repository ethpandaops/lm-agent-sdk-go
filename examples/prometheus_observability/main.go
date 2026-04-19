package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	// Create a dedicated Prometheus registry and wire it as the
	// SDK's meter provider via WithPrometheusRegisterer.
	reg := prometheus.NewRegistry()

	// Serve metrics on /metrics in the background.
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: ":9090", Handler: mux}
	go func() { _ = srv.ListenAndServe() }()
	defer func() { _ = srv.Close() }()

	fmt.Println("Prometheus metrics available at http://localhost:9090/metrics")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	for msg, err := range sdk.Query(ctx,
		sdk.Text("Write a two-line haiku about Go concurrency."),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithPrometheusRegisterer(reg),
		sdk.WithMaxTurns(2),
	) {
		if err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}
		exampleutil.DisplayMessage(msg)
	}

	fmt.Println("\nMetrics exported. Curl http://localhost:9090/metrics to inspect.")
}
