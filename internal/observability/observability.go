// Package observability provides OpenTelemetry metrics and tracing for the SDK.
package observability

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	agenterrclass "github.com/ethpandaops/agent-sdk-observability/errclass"
	"github.com/ethpandaops/agent-sdk-observability/semconv/genaiconv"
	"github.com/ethpandaops/agent-sdk-observability/semconv/httpconv"
	agenttracer "github.com/ethpandaops/agent-sdk-observability/tracer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	noopmetric "go.opentelemetry.io/otel/metric/noop"
	upstreamgenai "go.opentelemetry.io/otel/semconv/v1.40.0/genaiconv"
	upstreamhttp "go.opentelemetry.io/otel/semconv/v1.40.0/httpconv"
	"go.opentelemetry.io/otel/trace"

	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
)

const (
	instrumentationName    = "github.com/ethpandaops/lm-agent-sdk-go"
	instrumentationVersion = "0.1.0"
)

// providerName is the constant gen_ai.provider.name value for this SDK.
// Upstream's ProviderNameAttr is an open string type; the closed-set constants
// (ProviderNameOpenAI, ProviderNameAnthropic, ...) are a recommendation, not a
// restriction. "lmstudio" is semconv-conformant.
const providerName = upstreamgenai.ProviderNameAttr("lmstudio")

// Config holds the providers and logger used when constructing an Observer.
type Config struct {
	MeterProvider  metric.MeterProvider
	TracerProvider trace.TracerProvider
	Logger         *slog.Logger
}

// HTTPStatusError is returned by the transport for non-2xx HTTP responses.
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("lmstudio error status=%d body=%s", e.StatusCode, e.Body)
}

// Observer holds the meter/tracer and pre-constructed instruments.
type Observer struct {
	tracer  *agenttracer.Recorder
	classes *agenterrclass.Registry
	logger  *slog.Logger

	// Spec GenAI metrics — upstream instrument structs.
	opDuration upstreamgenai.ClientOperationDuration
	tokenUsage upstreamgenai.ClientTokenUsage

	// SDK-specific metrics — raw OTel.
	ttft                 metric.Float64Histogram
	httpRequestsTotal    metric.Int64Counter
	httpRequestDuration  metric.Float64Histogram
	toolCallsTotal       metric.Int64Counter
	toolCallDuration     metric.Float64Histogram
	checkpointOpsTotal   metric.Int64Counter
	rateLimitEvents      metric.Int64Counter
	hookDispatchDuration metric.Float64Histogram
}

// New returns an Observer. Providers default to noop when nil. The upstream
// instrument constructors can return errors; New propagates them.
func New(cfg Config) (*Observer, error) {
	mp := cfg.MeterProvider
	if mp == nil {
		mp = noopmetric.NewMeterProvider()
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	classes := newErrorRegistry()
	obs := &Observer{
		tracer:  agenttracer.New(cfg.TracerProvider, instrumentationName, instrumentationVersion, classes),
		classes: classes,
		logger:  logger,
	}

	meter := mp.Meter(instrumentationName, metric.WithInstrumentationVersion(instrumentationVersion))
	if err := obs.initMetrics(meter); err != nil {
		return nil, err
	}
	return obs, nil
}

// Noop returns an Observer with noop providers. A noop meter cannot produce a
// real instrument-construction error; a non-nil error here would be a library
// bug, so we panic rather than force every caller to handle an impossible case.
func Noop() *Observer {
	obs, err := New(Config{})
	if err != nil {
		panic("observability.Noop: " + err.Error())
	}
	return obs
}

// Classify returns the error.type class for err. Returns "" for nil.
func (o *Observer) Classify(err error) agenterrclass.Class {
	if err == nil {
		return ""
	}
	return o.classes.Classify(err)
}

// RegisterSentinel wires a sentinel error to a class.
func (o *Observer) RegisterSentinel(err error, class agenterrclass.Class) {
	if o == nil || err == nil {
		return
	}
	o.classes.RegisterSentinel(err, class)
}

// RegisterMatcher appends a custom matcher to the registry.
func (o *Observer) RegisterMatcher(matcher agenterrclass.Matcher) {
	if o == nil || matcher == nil {
		return
	}
	o.classes.RegisterMatcher(matcher)
}

// RecordOperationDuration records the spec gen_ai.client.operation.duration
// histogram with provider+operation positional attributes and optional
// request.model / error.type extras.
func (o *Observer) RecordOperationDuration(
	ctx context.Context,
	seconds float64,
	operationName upstreamgenai.OperationNameAttr,
	model string,
	class agenterrclass.Class,
) {
	errType := upstreamgenai.ErrorTypeOther
	if class != "" {
		errType = upstreamgenai.ErrorTypeAttr(class)
	}

	attrs := []attribute.KeyValue{o.opDuration.AttrErrorType(errType)}
	if model != "" {
		attrs = append(attrs, o.opDuration.AttrRequestModel(model))
	}
	o.opDuration.Record(ctx, seconds, operationName, providerName, attrs...)
}

// RecordTokenUsage records the spec gen_ai.client.token.usage histogram.
// Emits a span event for "thinking" tokens.
func (o *Observer) RecordTokenUsage(
	ctx context.Context,
	tokens int64,
	tokenType upstreamgenai.TokenTypeAttr,
	operationName upstreamgenai.OperationNameAttr,
	model string,
) {
	if tokens <= 0 {
		return
	}
	attrs := []attribute.KeyValue{}
	if model != "" {
		attrs = append(attrs, o.tokenUsage.AttrRequestModel(model))
	}
	o.tokenUsage.Record(ctx, tokens, operationName, providerName, tokenType, attrs...)
	if tokenType == upstreamgenai.TokenTypeAttr("thinking") {
		trace.SpanFromContext(ctx).AddEvent("thinking_tokens",
			trace.WithAttributes(ThinkingTokens(tokens)))
	}
}

// RecordTTFT records time-to-first-token on the SDK-local histogram.
func (o *Observer) RecordTTFT(ctx context.Context, seconds float64, model string) {
	attrs := []attribute.KeyValue{}
	if model != "" {
		attrs = append(attrs, genaiconv.RequestModel(model))
	}
	o.ttft.Record(ctx, seconds, metric.WithAttributes(attrs...))
}

// RecordHTTPRequest increments the HTTP request counter.
func (o *Observer) RecordHTTPRequest(ctx context.Context, statusClass string, isRetry bool) {
	o.httpRequestsTotal.Add(ctx, 1,
		metric.WithAttributes(StatusClass(statusClass), Retry(isRetry)))
}

// RecordHTTPRequestDuration records the HTTP request duration histogram.
func (o *Observer) RecordHTTPRequestDuration(ctx context.Context, seconds float64, statusClass string, isRetry bool) {
	o.httpRequestDuration.Record(ctx, seconds,
		metric.WithAttributes(StatusClass(statusClass), Retry(isRetry)))
}

// RecordRateLimitEvent increments the rate-limit counter and emits a span event.
func (o *Observer) RecordRateLimitEvent(ctx context.Context) {
	o.rateLimitEvents.Add(ctx, 1)
	trace.SpanFromContext(ctx).AddEvent("rate_limit")
}

// RecordToolCall increments the tool-call counter.
func (o *Observer) RecordToolCall(ctx context.Context, toolName, outcome string) {
	o.toolCallsTotal.Add(ctx, 1,
		metric.WithAttributes(genaiconv.ToolName(toolName), Outcome(outcome)))
}

// RecordToolCallDuration records the tool-call duration histogram.
func (o *Observer) RecordToolCallDuration(ctx context.Context, seconds float64, toolName string) {
	o.toolCallDuration.Record(ctx, seconds,
		metric.WithAttributes(genaiconv.ToolName(toolName)))
}

// RecordCheckpointOp increments the checkpoint-ops counter and emits a span event.
func (o *Observer) RecordCheckpointOp(ctx context.Context, op, outcome string) {
	o.checkpointOpsTotal.Add(ctx, 1,
		metric.WithAttributes(CheckpointOp(op), Outcome(outcome)))
	trace.SpanFromContext(ctx).AddEvent("checkpoint",
		trace.WithAttributes(CheckpointOp(op), Outcome(outcome)))
}

// RecordHookDuration records the hook dispatch duration histogram.
func (o *Observer) RecordHookDuration(ctx context.Context, seconds float64, event, outcome string) {
	o.hookDispatchDuration.Record(ctx, seconds,
		metric.WithAttributes(HookEvent(event), Outcome(outcome)))
}

// StartQuerySpan opens the top-level span for a Query()/Client.Query() call.
func (o *Observer) StartQuerySpan(
	ctx context.Context,
	operationName upstreamgenai.OperationNameAttr,
	model string,
	conversationID string,
) (context.Context, *agenttracer.Span) {
	attrs := []attribute.KeyValue{
		genaiconv.OperationName(operationName),
		genaiconv.ProviderName(providerName),
	}
	if model != "" {
		attrs = append(attrs, genaiconv.RequestModel(model))
	}
	if conversationID != "" {
		attrs = append(attrs, genaiconv.ConversationID(conversationID))
	}
	return o.tracer.Start(ctx,
		genaiconv.SpanName(operationName, model),
		trace.SpanKindClient, attrs...)
}

// StartHTTPSpan opens a child span for a vLLM HTTP request. The
// transport only issues POSTs, so method is hardcoded.
func (o *Observer) StartHTTPSpan(
	ctx context.Context,
	endpoint string,
) (context.Context, *agenttracer.Span) {
	return o.tracer.Start(ctx, "lmstudio.http.request", trace.SpanKindClient,
		httpconv.RequestMethod(upstreamhttp.RequestMethodPost),
		httpconv.URLPath(endpoint),
	)
}

// StartToolSpan opens a child span for a tool invocation. Spec-conformant
// "execute_tool {name}" span name and gen_ai.operation.name=execute_tool
// attribute per GenAI semantic conventions. provider.name is set so tool
// spans carry the same filterable labels as query spans. callID is optional;
// pass "" if the tool call has no upstream identifier.
func (o *Observer) StartToolSpan(ctx context.Context, toolName, callID string) (context.Context, *agenttracer.Span) {
	attrs := []attribute.KeyValue{
		genaiconv.OperationName(upstreamgenai.OperationNameExecuteTool),
		genaiconv.ProviderName(providerName),
		genaiconv.ToolName(toolName),
	}
	if callID != "" {
		attrs = append(attrs, genaiconv.ToolCallID(callID))
	}
	return o.tracer.Start(ctx,
		genaiconv.SpanName(upstreamgenai.OperationNameExecuteTool, toolName),
		trace.SpanKindInternal, attrs...,
	)
}

// StartHookSpan opens a child span for a hook dispatch.
func (o *Observer) StartHookSpan(ctx context.Context, event string) (context.Context, *agenttracer.Span) {
	return o.tracer.Start(ctx, "lmstudio.hook.dispatch", trace.SpanKindInternal,
		HookEvent(event))
}

// StatusClassOf returns "2xx"/"3xx"/"4xx"/"5xx"/"other" for an HTTP status code.
func StatusClassOf(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	case code >= 500 && code < 600:
		return "5xx"
	default:
		return "other"
	}
}

func newErrorRegistry() *agenterrclass.Registry {
	reg := agenterrclass.New()
	reg.RegisterDefaults() // context.Canceled / context.DeadlineExceeded

	// ErrOperationCancelled is not the stdlib context.Canceled, so errors.Is
	// does not bridge them. This registration is intentional alongside
	// RegisterDefaults().
	reg.RegisterSentinel(sdkerrors.ErrOperationCancelled, agenterrclass.Canceled)
	reg.RegisterSentinel(sdkerrors.ErrRequestTimeout, agenterrclass.Timeout)

	reg.RegisterMatcher(func(err error) (agenterrclass.Class, bool) {
		var typed *sdkerrors.ToolPermissionDeniedError
		if errors.As(err, &typed) {
			return agenterrclass.PermissionDenied, true
		}
		return "", false
	})
	reg.RegisterMatcher(func(err error) (agenterrclass.Class, bool) {
		var typed *sdkerrors.UnsupportedHookEventError
		if errors.As(err, &typed) {
			return ClassUnsupportedHookEvent, true
		}
		return "", false
	})
	reg.RegisterMatcher(func(err error) (agenterrclass.Class, bool) {
		var typed *sdkerrors.UnsupportedHookOutputError
		if errors.As(err, &typed) {
			return ClassUnsupportedHookOutput, true
		}
		return "", false
	})
	reg.RegisterMatcher(func(err error) (agenterrclass.Class, bool) {
		var typed *HTTPStatusError
		if !errors.As(err, &typed) {
			return "", false
		}
		switch {
		case typed.StatusCode == 429:
			return agenterrclass.RateLimited, true
		case typed.StatusCode == 401 || typed.StatusCode == 403:
			return agenterrclass.Auth, true
		case typed.StatusCode >= 500:
			return agenterrclass.Upstream5xx, true
		default:
			return agenterrclass.InvalidRequest, true
		}
	})
	reg.RegisterMatcher(func(err error) (agenterrclass.Class, bool) {
		joined := strings.ToLower(err.Error())
		switch {
		case strings.Contains(joined, "rate limit"):
			return agenterrclass.RateLimited, true
		case strings.Contains(joined, "unauthorized"),
			strings.Contains(joined, "forbidden"),
			strings.Contains(joined, "auth"):
			return agenterrclass.Auth, true
		default:
			return "", false
		}
	})
	return reg
}
