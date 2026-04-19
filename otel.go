package lmsdk

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	agenterrclass "github.com/ethpandaops/agent-sdk-observability/errclass"
	genaiconv "github.com/ethpandaops/agent-sdk-observability/semconv/genaiconv"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	upstreamgenai "go.opentelemetry.io/otel/semconv/v1.40.0/genaiconv"
	"go.opentelemetry.io/otel/trace"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/message"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/observability"
)

// otelRecorder adapts *observability.Observer to the config.SessionMetricsRecorder
// interface used by the SDK runtime. It threads per-session state (TTFT,
// model caching) alongside the shared Observer.
//
// Tool span correlation is handled directly by the QueryRunner for precise
// timing and error classification (denied/error/ok outcomes). The recorder
// focuses on message-level observation: TTFT, token usage, operation
// duration, and query span enrichment.
type otelRecorder struct {
	obs *observability.Observer

	// TTFT: set by MarkQueryStart, cleared after first AssistantMessage records.
	queryStartNs atomic.Int64
	ttftDone     atomic.Bool

	// Cached model name from the first AssistantMessage -- used as the
	// request.model label when observing events that lack explicit model info.
	modelMu sync.RWMutex
	model   string
}

// Compile-time checks.
var (
	_ config.SessionMetricsRecorder = (*otelRecorder)(nil)
	_ config.QueryLifecycleNotifier = (*otelRecorder)(nil)
)

// newOTelRecorder creates a recorder from typed OTel providers.
// Returns nil if both providers are nil, ensuring zero overhead for unconfigured sessions.
func newOTelRecorder(
	meterProvider metric.MeterProvider,
	tracerProvider trace.TracerProvider,
	logger *slog.Logger,
) *otelRecorder {
	if meterProvider == nil && tracerProvider == nil {
		return nil
	}

	obs, err := observability.New(observability.Config{
		MeterProvider:  meterProvider,
		TracerProvider: tracerProvider,
		Logger:         logger,
	})
	if err != nil {
		// The shared library only fails on real meter provider errors
		// (custom providers). Fall back to noop so the SDK remains usable.
		return nil
	}

	return &otelRecorder{obs: obs}
}

// setModel caches the first observed model name.
func (r *otelRecorder) setModel(model string) {
	if model == "" {
		return
	}

	r.modelMu.RLock()
	current := r.model
	r.modelMu.RUnlock()

	if current != "" {
		return
	}

	r.modelMu.Lock()
	if r.model == "" {
		r.model = model
	}
	r.modelMu.Unlock()
}

// currentModel returns the cached model name (may be empty).
func (r *otelRecorder) currentModel() string {
	r.modelMu.RLock()
	defer r.modelMu.RUnlock()

	return r.model
}

// markQueryStart captures the query start time for TTFT recording.
func (r *otelRecorder) markQueryStart() {
	r.queryStartNs.Store(time.Now().UnixNano())
	r.ttftDone.Store(false)
}

// MarkQueryStart implements config.QueryLifecycleNotifier by recording the
// query start time for TTFT measurement.
func (r *otelRecorder) MarkQueryStart() {
	r.markQueryStart()
}

// recordTTFTOnce records TTFT on the first qualifying observation.
func (r *otelRecorder) recordTTFTOnce(ctx context.Context, model string) {
	start := r.queryStartNs.Load()
	if start == 0 {
		return
	}

	if !r.ttftDone.CompareAndSwap(false, true) {
		return
	}

	elapsed := time.Since(time.Unix(0, start)).Seconds()
	r.obs.RecordTTFT(ctx, elapsed, model)
}

// Observe records metrics from a parsed message, dispatching to type-specific handlers.
// The context enables trace correlation and exemplar propagation.
func (r *otelRecorder) Observe(ctx context.Context, msg message.Message) {
	if r == nil || msg == nil {
		return
	}

	switch typed := msg.(type) {
	case *message.AssistantMessage:
		r.observeAssistant(ctx, typed)
	case *message.ResultMessage:
		r.observeResult(ctx, typed)
	}
}

// observeAssistant records TTFT on the first assistant message and caches
// the model name. Tool span correlation is handled directly by QueryRunner.
func (r *otelRecorder) observeAssistant(ctx context.Context, msg *message.AssistantMessage) {
	if msg == nil {
		return
	}

	r.setModel(msg.Model)

	// Check for non-empty content (text or tool) before recording TTFT.
	for _, block := range msg.Content {
		switch block.(type) {
		case *message.TextBlock, *message.ToolUseBlock:
			r.recordTTFTOnce(ctx, msg.Model)

			return
		}
	}
}

// enrichSpanFromResult sets response attributes and error status on the
// query/session span carried by ctx. Called once when a ResultMessage arrives.
func (r *otelRecorder) enrichSpanFromResult(ctx context.Context, result *message.ResultMessage) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	// Response model.
	model := r.currentModel()
	if model != "" {
		span.SetAttributes(genaiconv.ResponseModel(model))
	}

	// Stop reason -> gen_ai.response.finish_reasons.
	if result.StopReason != nil && *result.StopReason != "" {
		span.SetAttributes(observability.FinishReasons(*result.StopReason))
	}

	// Error status.
	if result.IsError {
		class := r.classifyResult(result)
		if class != "" {
			span.SetAttributes(class.Attr())
			span.SetStatus(codes.Error, string(class))
		} else {
			span.SetStatus(codes.Error, "unknown error")
		}
	}
}

// observeResult records token usage and operation duration from a terminal result.
func (r *otelRecorder) observeResult(ctx context.Context, result *message.ResultMessage) {
	if result == nil {
		return
	}

	// Enrich the query/session span with response attributes and error status.
	r.enrichSpanFromResult(ctx, result)

	model := r.currentModel()

	// Token usage from the result.
	if result.Usage != nil {
		if result.Usage.InputTokens > 0 {
			r.obs.RecordTokenUsage(ctx, int64(result.Usage.InputTokens),
				upstreamgenai.TokenTypeInput, upstreamgenai.OperationNameChat, model)
		}

		if result.Usage.OutputTokens > 0 {
			r.obs.RecordTokenUsage(ctx, int64(result.Usage.OutputTokens),
				upstreamgenai.TokenTypeOutput, upstreamgenai.OperationNameChat, model)
		}

		if result.Usage.CachedInputTokens > 0 {
			r.obs.RecordTokenUsage(ctx, int64(result.Usage.CachedInputTokens),
				upstreamgenai.TokenTypeAttr("cache_read"), upstreamgenai.OperationNameChat, model)
		}

		if result.Usage.ReasoningOutputTokens > 0 {
			r.obs.RecordTokenUsage(ctx, int64(result.Usage.ReasoningOutputTokens),
				upstreamgenai.TokenTypeAttr("thinking"), upstreamgenai.OperationNameChat, model)
		}
	}

	// Operation duration with model and error.type labels.
	if result.DurationMs > 0 {
		duration := time.Duration(result.DurationMs) * time.Millisecond
		class := r.classifyResult(result)

		r.obs.RecordOperationDuration(ctx, duration.Seconds(),
			upstreamgenai.OperationNameChat, model, class)
	}
}

// classifyResult maps a ResultMessage to an errclass.Class. Uses the shared
// registry for string-matching errors and falls back to subtype-based mapping.
func (r *otelRecorder) classifyResult(result *message.ResultMessage) agenterrclass.Class {
	if !result.IsError {
		return ""
	}

	// Try to classify from the result text.
	if result.Result != nil && *result.Result != "" {
		if class := r.obs.Classify(errors.New(*result.Result)); class != "" && class != agenterrclass.Unknown {
			return class
		}
	}

	// Try to classify from the stop reason.
	if result.StopReason != nil {
		if class := r.obs.Classify(errors.New(*result.StopReason)); class != "" && class != agenterrclass.Unknown {
			return class
		}
	}

	// Fall back to subtype-based mapping.
	switch result.Subtype {
	case "error_max_budget_usd":
		return agenterrclass.Class("max_budget_usd")
	case "error":
		return observability.ClassExecution
	}

	return observability.ClassExecution
}

// initMetricsRecorder creates and stores the OTel recorder on options if providers are configured.
// This is called at runtime entry points (Query, QueryStream, Client.Start).
func initMetricsRecorder(options *config.Options) {
	if options == nil || options.MetricsRecorder != nil {
		return
	}

	mp := options.MeterProvider
	tp := options.TracerProvider

	// Bridge Prometheus if no direct MeterProvider is set.
	if mp == nil && options.PrometheusRegisterer != nil {
		var err error

		mp, err = observability.NewPrometheusMeterProvider(options.PrometheusRegisterer)
		if err != nil {
			if options.Logger != nil {
				options.Logger.Warn("failed to create prometheus meter provider, falling back to noop",
					slog.String("error", err.Error()),
				)
			}
		}
	}

	recorder := newOTelRecorder(mp, tp, options.Logger)
	if recorder != nil {
		options.MetricsRecorder = recorder
		options.Observer = recorder.obs
	}
}
