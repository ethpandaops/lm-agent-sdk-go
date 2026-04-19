package observability

import (
	"fmt"

	"go.opentelemetry.io/otel/metric"
	upstreamgenai "go.opentelemetry.io/otel/semconv/v1.40.0/genaiconv"
)

func (o *Observer) initMetrics(meter metric.Meter) error {
	var err error

	// Spec GenAI metrics — upstream instrument structs.
	if o.opDuration, err = upstreamgenai.NewClientOperationDuration(meter); err != nil {
		return fmt.Errorf("gen_ai.client.operation.duration: %w", err)
	}
	if o.tokenUsage, err = upstreamgenai.NewClientTokenUsage(meter); err != nil {
		return fmt.Errorf("gen_ai.client.token.usage: %w", err)
	}

	// TTFT — spec metric (Development status). The upstream genaiconv package
	// at v1.40.0 only ships `gen_ai.server.time_to_first_token`; the
	// client-side variant is defined in the spec but not yet exposed as an
	// instrument struct. Construct it via raw meter so we emit the
	// spec-conformant name today and can switch to the upstream struct when
	// it lands.
	if o.ttft, err = meter.Float64Histogram(
		"gen_ai.client.operation.time_to_first_chunk",
		metric.WithDescription("Time to receive the first chunk in a streaming GenAI operation"),
		metric.WithUnit("s"),
	); err != nil {
		return fmt.Errorf("gen_ai.client.operation.time_to_first_chunk: %w", err)
	}

	// SDK-specific duration histograms. OTel convention: units belong in
	// WithUnit, not in the metric name. The Prometheus exporter's default
	// translator appends `_seconds` on emit.
	if o.httpRequestDuration, err = meter.Float64Histogram(
		"lmstudio.http_request_duration",
		metric.WithDescription("Duration of HTTP requests to vLLM API"),
		metric.WithUnit("s"),
	); err != nil {
		return fmt.Errorf("lmstudio.http_request_duration: %w", err)
	}
	if o.toolCallDuration, err = meter.Float64Histogram(
		"lmstudio.tool_call_duration",
		metric.WithDescription("Duration of tool call executions"),
		metric.WithUnit("s"),
	); err != nil {
		return fmt.Errorf("lmstudio.tool_call_duration: %w", err)
	}
	if o.hookDispatchDuration, err = meter.Float64Histogram(
		"lmstudio.hook_dispatch_duration",
		metric.WithDescription("Duration of hook dispatches"),
		metric.WithUnit("s"),
	); err != nil {
		return fmt.Errorf("lmstudio.hook_dispatch_duration: %w", err)
	}

	// Counters — `_total` suffix preserved by the Prometheus exporter.
	if o.httpRequestsTotal, err = meter.Int64Counter(
		"lmstudio.http_requests_total",
		metric.WithDescription("Total number of HTTP requests to vLLM API"),
		metric.WithUnit("{request}"),
	); err != nil {
		return fmt.Errorf("lmstudio.http_requests_total: %w", err)
	}
	if o.toolCallsTotal, err = meter.Int64Counter(
		"lmstudio.tool_calls_total",
		metric.WithDescription("Total number of tool call invocations"),
		metric.WithUnit("{call}"),
	); err != nil {
		return fmt.Errorf("lmstudio.tool_calls_total: %w", err)
	}
	if o.checkpointOpsTotal, err = meter.Int64Counter(
		"lmstudio.checkpoint_operations_total",
		metric.WithDescription("Total number of checkpoint operations"),
		metric.WithUnit("{operation}"),
	); err != nil {
		return fmt.Errorf("lmstudio.checkpoint_operations_total: %w", err)
	}
	if o.rateLimitEvents, err = meter.Int64Counter(
		"lmstudio.rate_limit_events_total",
		metric.WithDescription("Total number of rate limit events (HTTP 429)"),
		metric.WithUnit("{event}"),
	); err != nil {
		return fmt.Errorf("lmstudio.rate_limit_events_total: %w", err)
	}

	return nil
}
